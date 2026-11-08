package metrics

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Windows — окна скользящего TBSLO.
var Windows = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, time.Hour}

// Minute — шаг семплирования TBSLO: «минута» = преобладающий исход всех
// проб за календарную минуту.
const Minute = time.Minute

type minuteBucket struct {
	start time.Time
	good  int
	bad   int
}

// Prober раз в interval пробит /health сервиса и раз в минуту публикует TBSLO:
// долю хороших минут в скользящем окне.
type Prober struct {
	service  string
	probeURL string
	method   string // GET для /health, POST для gateway (login без тела даёт 400)
	interval time.Duration
	log      *slog.Logger

	buckets []minuteBucket // завершённые минуты
	pending *minuteBucket  // текущая минута
}

var (
	SLOProbeUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wave_slo_probe_up",
		Help: "1 if the last SLO probe of the backend succeeded, else 0",
	}, []string{"service"})

	SLOWindowTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wave_slo_window_total",
		Help: "Number of probed minutes in the sliding window (TBSLO denominator)",
	}, []string{"service", "window"})

	SLOWindowGood = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wave_slo_window_good",
		Help: "Number of good minutes in the sliding window (TBSLO numerator)",
	}, []string{"service", "window"})
)

func NewProber(service, probeURL string, interval time.Duration, log *slog.Logger) *Prober {
	return &Prober{service: service, probeURL: probeURL, method: http.MethodGet, interval: interval, log: log}
}

// NewProberMethod — как NewProber, но с явным HTTP-методом.
// Для gateway нужен POST: GET /api/auth/login отвечает 405.
func NewProberMethod(method, service, probeURL string, interval time.Duration, log *slog.Logger) *Prober {
	p := NewProber(service, probeURL, interval, log)
	p.method = method
	return p
}

// Run запускает пробер в фоне. Вызывается один раз из ServeWithProber.
// Первая проба делается сразу, чтобы wave_slo_probe_up появился без ожидания
// целого интервала (иначе график пуст первые 60с после пода).
func (p *Prober) Run() {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		now := time.Now()
		p.record(now, p.probe())
		p.publish(now)

		for {
			now := <-ticker.C
			p.record(now, p.probe())
			p.publish(now)
			p.trim(now)
		}
	}()
}

// record сохраняет исход пробы: gauge для алертов и минуту для TBSLO.
func (p *Prober) record(now time.Time, good bool) {
	SLOProbeUp.WithLabelValues(p.service).Set(boolFloat(good))
	p.observe(now, good)
}

// observe учитывает одну пробу в минутном бакете.
func (p *Prober) observe(now time.Time, good bool) {
	start := now.Truncate(Minute)
	if p.pending == nil || !p.pending.start.Equal(start) {
		p.flush()
		p.pending = &minuteBucket{start: start}
	}
	if good {
		p.pending.good++
	} else {
		p.pending.bad++
	}
}

// flush фиксирует завершившуюся минуту. Минута «хорошая», если хороших проб
// не меньше половины.
func (p *Prober) flush() {
	if p.pending == nil {
		return
	}
	p.buckets = append(p.buckets, minuteBucket{
		start: p.pending.start,
		good:  boolToInt(p.minuteGood(*p.pending)),
	})
	p.pending = nil
}

func (p *Prober) minuteGood(b minuteBucket) bool {
	return b.good > 0 && b.good >= b.bad
}

func (p *Prober) probe() bool {
	req, err := http.NewRequest(p.method, p.probeURL, nil)
	if err != nil {
		return false
	}
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		p.log.Warn("slo probe failed", "service", p.service, "url", p.probeURL, "error", err)
		return false
	}
	defer resp.Body.Close()

	// 4xx — живой сервис, который осознанно отказал (например, gateway
	// на POST /api/auth/login без тела отвечает 400). Живость определяет
	// способность ответить, а не код. Тот же принцип, что в Track/Classify.
	class := Classify(resp.StatusCode)
	ok := class != OutcomeBad
	p.log.Info("slo probe", "service", p.service, "status", resp.StatusCode, "class", class, "ok", ok)
	return ok
}

// publish пишет TBSLO по всем окнам: завершённые минуты + текущая.
// Минута входит в окно W, если её начало не старше now-W.
func (p *Prober) publish(now time.Time) {
	for _, w := range Windows {
		cut := now.Add(-w)
		var n, g int
		for _, b := range p.buckets {
			if b.start.Before(cut) {
				continue
			}
			n++
			g += b.good
		}
		if p.pending != nil && !p.pending.start.Before(cut) {
			n++
			g += boolToInt(p.minuteGood(*p.pending))
		}
		// window — сокращённое имя окна (5m, не 5m0s):
		// запросы в rules.txt и демо используют window="5m".
		labels := prometheus.Labels{"service": p.service, "window": shortWindow(w)}
		SLOWindowTotal.With(labels).Set(float64(n))
		SLOWindowGood.With(labels).Set(float64(g))
	}
}

func (p *Prober) trim(now time.Time) {
	cut := now.Add(-Windows[len(Windows)-1])
	i := 0
	for ; i < len(p.buckets); i++ {
		if p.buckets[i].start.After(cut) {
			break
		}
	}
	p.buckets = p.buckets[i:]
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// shortWindow переводит 5m0s в 5m. Окно в лейбле должно совпадать
// с тем, что написано в rules.txt и query.txt.
func shortWindow(w time.Duration) string {
	if w%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(w/time.Hour))
	}
	if w%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(w/time.Minute))
	}
	return w.String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
