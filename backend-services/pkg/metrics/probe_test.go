package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testGauge(v *prometheus.GaugeVec, service, window string) float64 {
	return testutil.ToFloat64(v.WithLabelValues(service, window))
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
	if got := testGauge(SLOWindowTotal, "feed", "30m0s"); got != 10 {
		t.Errorf("30m total = %v, want 10", got)
	}
	if got := testGauge(SLOWindowGood, "feed", "30m0s"); got != 4 {
		t.Errorf("30m good = %v, want 4", got)
	}

	// Окно 5m: минуты now-4..now-1 (4 шт.) + текущая = 5.
	// Хорошие из них — now-3..now-1 и текущая = 4.
	if got := testGauge(SLOWindowTotal, "feed", "5m0s"); got != 5 {
		t.Errorf("5m total = %v, want 5", got)
	}
	if got := testGauge(SLOWindowGood, "feed", "5m0s"); got != 4 {
		t.Errorf("5m good = %v, want 4", got)
	}

	// Окно 1m: только текущая минута.
	if got := testGauge(SLOWindowTotal, "feed", "1m0s"); got != 1 {
		t.Errorf("1m total = %v, want 1", got)
	}
	if got := testGauge(SLOWindowGood, "feed", "1m0s"); got != 1 {
		t.Errorf("1m good = %v, want 1", got)
	}
}
