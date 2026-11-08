package metrics

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testGauge(v *prometheus.GaugeVec, service, window string) float64 {
	return testutil.ToFloat64(v.WithLabelValues(service, window))
}

func TestProberTreats4xxAsAlive(t *testing.T) {
	// Регресс: gateway на POST /api/auth/login без тела отвечает 400.
	// Если считать живым только 2xx/3xx, wave_slo_probe_up{service="gateway"}
	// навсегда 0 и WaveAllReplicasDown срабатывает ложно.
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("probe method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewProberMethod(http.MethodPost, "gateway", srv.URL+"/ok", time.Minute,
		slog.New(slog.DiscardHandler))
	if !p.probe() {
		t.Error("probe of 400 = false, want true (service is alive)")
	}
}

func TestProberTreats5xxAsDead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewProber("feed", srv.URL, time.Minute, slog.New(slog.DiscardHandler))
	if p.probe() {
		t.Error("probe of 500 = true, want false")
	}
}

func TestProberRecordSetsProbeUp(t *testing.T) {
	// Регресс: SLOProbeUp была объявлена, но никто её не устанавливал,
	// поэтому wave_slo_probe_up отсутствовала в /metrics и алерты
	// на неё никогда не срабатывали.
	p := NewProber("feed", "", time.Minute, slog.New(slog.DiscardHandler))
	now := time.Now()

	p.record(now, true)
	if got := testutil.ToFloat64(SLOProbeUp.WithLabelValues("feed")); got != 1 {
		t.Errorf("probe_up after good probe = %v, want 1", got)
	}

	p.record(now, false)
	if got := testutil.ToFloat64(SLOProbeUp.WithLabelValues("feed")); got != 0 {
		t.Errorf("probe_up after failed probe = %v, want 0", got)
	}
}

func TestShortWindow(t *testing.T) {
	// Лейбл должен совпадать с тем, что написано в rules.txt/query.txt.
	cases := map[time.Duration]string{
		time.Minute:      "1m",
		5 * time.Minute:  "5m",
		30 * time.Minute: "30m",
		time.Hour:        "1h",
	}
	for d, want := range cases {
		if got := shortWindow(d); got != want {
			t.Errorf("shortWindow(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestProberMinuteGoodness(t *testing.T) {
	p := NewProber("feed", "", time.Minute, nil)

	// Минута, где проб больше, чем плохих, — «хорошая».
	p.observe(time.Unix(1000, 0), true)
	p.observe(time.Unix(1000, 30), true)
	p.observe(time.Unix(1000, 60), false)
	p.flush()

	// Минута, где плохих больше, — «плохая».
	p.observe(time.Unix(1060, 0), false)
	p.observe(time.Unix(1060, 30), false)
	p.observe(time.Unix(1060, 60), true)
	p.flush()

	if len(p.buckets) != 2 {
		t.Fatalf("buckets = %d, want 2", len(p.buckets))
	}
	if p.buckets[0].good != 1 {
		t.Errorf("first minute good = %d, want 1", p.buckets[0].good)
	}
	if p.buckets[1].good != 0 {
		t.Errorf("second minute good = %d, want 0", p.buckets[1].good)
	}
}

func TestProberFlipsMinuteOnBoundary(t *testing.T) {
	p := NewProber("feed", "", time.Minute, nil)

	p.observe(time.Unix(120, 0), true)  // минута 120
	p.observe(time.Unix(180, 0), false) // минута 180 — переход должен зафиксировать предыдущую
	if len(p.buckets) != 1 {
		t.Fatalf("buckets = %d, want 1 (предыдущая минута зафиксирована)", len(p.buckets))
	}
	if p.buckets[0].good != 1 {
		t.Errorf("closed minute good = %d, want 1", p.buckets[0].good)
	}
	if p.pending == nil || p.pending.start.Unix() != 180 {
		t.Errorf("pending minute = %v, want 180", p.pending)
	}
}

func TestProberPublishWindows(t *testing.T) {
	p := NewProber("feed", "", time.Minute, nil)

	// Тест напрямую задаёт `now`, поэтому `at` в прошлом — как в реальности.
	// Бакеты создаются относительно реального времени, чтобы publish не обнулил их.
	// Проба в реальности приходит не в начале минуты, а внутри неё.
	now := time.Now().Truncate(time.Minute).Add(30 * time.Second)
	startOfMinute := now.Truncate(time.Minute)
	// Завершённые минуты: now-9 .. now-1.
	// Хорошие только последние 3 из них: now-3, now-2, now-1.
	for k := 9; k >= 1; k-- {
		p.buckets = append(p.buckets, minuteBucket{
			start: startOfMinute.Add(-time.Duration(k) * time.Minute),
			good:  boolToInt(k <= 3),
		})
	}
	// Текущая минута ещё не завершена, но уже учтена и тоже хорошая.
	p.pending = &minuteBucket{start: startOfMinute, good: 2, bad: 1}

	p.publish(now)

	// Окно 30m: 9 завершённых + текущая = 10, хороших 3+1 = 4.
	if got := testGauge(SLOWindowTotal, "feed", "30m"); got != 10 {
		t.Errorf("30m total = %v, want 10", got)
	}
	if got := testGauge(SLOWindowGood, "feed", "30m"); got != 4 {
		t.Errorf("30m good = %v, want 4", got)
	}

	// Окно 5m: минуты now-4..now-1 (4 шт.) + текущая = 5.
	// Хорошие из них — now-3..now-1 и текущая = 4.
	if got := testGauge(SLOWindowTotal, "feed", "5m"); got != 5 {
		t.Errorf("5m total = %v, want 5", got)
	}
	if got := testGauge(SLOWindowGood, "feed", "5m"); got != 4 {
		t.Errorf("5m good = %v, want 4", got)
	}

	// Окно 1m: только текущая минута.
	if got := testGauge(SLOWindowTotal, "feed", "1m"); got != 1 {
		t.Errorf("1m total = %v, want 1", got)
	}
	if got := testGauge(SLOWindowGood, "feed", "1m"); got != 1 {
		t.Errorf("1m good = %v, want 1", got)
	}
}
