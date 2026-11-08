package metrics

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	prometheus.MustRegister(SLORequests, SLODuration, SLOProbeUp, SLOWindowTotal, SLOWindowGood)
}

// Serve запускает HTTP-сервер на отдельном порту для /metrics.
// Пример: Serve(":9090") — метрики будут на http://<pod>:9090/metrics
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(addr, mux)
}

// ServeWithProber — как Serve, но дополнительно запускает TBSLO-пробер,
// который раз в interval проверяет healthURL и публикует wave_slo_window_*.
func ServeWithProber(addr, service, healthURL string, interval time.Duration, log *slog.Logger) error {
	NewProber(service, healthURL, interval, log).Run()
	return Serve(addr)
}
