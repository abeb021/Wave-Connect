package metrics

import (
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
	return &Prober{service: service, probeURL: probeURL, interval: interval, log: log}
}

// Run запускает пробер в фоне. Вызывается один раз из ServeWithProber.
// Первая проба делается сразу, чтобы wave_slo_probe_up появился без ожидания
// целого интервала (иначе график пуст первые 60с после пода).
func (p *Prober) Run() {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		now := time.Now()
		p.observe(now, p.probe())
		p.publish(now)

		for {
			now := <-ticker.C
			p.observe(now, p.probe())
			p.publish(now)
			p.trim(now)
		}
	}()
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
	req, err := http.NewRequest(http.MethodGet, p.probeURL, nil)
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

	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	p.log.Info("slo probe", "service", p.service, "status", resp.StatusCode, "ok", ok)
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
		labels := prometheus.Labels{"service": p.service, "window": w.String()}
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
