package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Serve запускает HTTP-сервер на отдельном порту для /metrics.
// Пример: Serve(":9090") — метрики будут на http://<pod>:9090/metrics
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(addr, mux)
}