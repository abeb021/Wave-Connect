package metrics

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Outcome — класс исхода запроса для SLI.
//
//	good    — сервис отработал запрос (2xx/3xx);
//	neutral — осознанный отказ клиенту (4xx): неверный пароль, битый JWT, неизвестный id.
//	          Это НЕ деградация сервиса: знаменатель RBSLO растёт, а «провалов» — нет;
//	bad     — ошибка сервиса (5xx) или отсутствие ответа (timeout / connection refused → 502).
type Outcome string

const (
	OutcomeGood    Outcome = "good"
	OutcomeNeutral Outcome = "neutral"
	OutcomeBad     Outcome = "bad"
)

// Classify переводит HTTP-статус в Outcome. status == 0 — ответа не было.
func Classify(status int) Outcome {
	switch {
	case status == 0:
		return OutcomeBad
	case status >= 200 && status < 400:
		return OutcomeGood
	case status >= 400 && status < 500:
		return OutcomeNeutral
	default:
		return OutcomeBad
	}
}

var (
	// SLORequests — числитель и знаменатель RBSLO в одном счётчике.
	//	всего  = sum by (service) (increase(wave_slo_requests_total[5m]))
	//	плохих = sum by (service) (increase(wave_slo_requests_total{class="bad"}[5m]))
	SLORequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wave_slo_requests_total",
			Help: "HTTP requests classified for SLI: good (2xx/3xx), neutral (4xx), bad (5xx or no response)",
		},
		[]string{"service", "class", "reason"},
	)

	// SLODuration — латентность в секундах для latency-SLI (p95).
	SLODuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "wave_slo_request_duration_seconds",
			Help:    "HTTP request latency for latency-based SLI",
			Buckets: []float64{.05, .1, .25, .5, .75, 1, 2, 5},
		},
		[]string{"service", "class"},
	)
)

// Track считает исход запроса на ЭТОМ процессе для слоя defaultService.
//
// Важно: X-SLO-Service с gateway сюда не читается — иначе один и тот же
// запрос посчитался бы дважды (на gateway и внутри сервиса), и сумма по
// кластеру раздулась бы вдвое. PromQL сам суммирует по репликам.
func Track(defaultService string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if defaultService == "" {
			next.ServeHTTP(w, r) // служебный процесс без SLI
			return
		}

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		status := sw.statusCode()
		class := Classify(status)
		SLORequests.WithLabelValues(defaultService, string(class), reason(status, class)).Inc()
		SLODuration.WithLabelValues(defaultService, string(class)).Observe(time.Since(start).Seconds())
	})
}

// reason — низкокардинальное объяснение исхода для дашборда.
func reason(status int, class Outcome) string {
	switch status {
	case 0:
		return "no_response"
	case 400:
		return "bad_request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 409:
		return "conflict"
	case 502:
		return "bad_gateway"
	case 503:
		return "unavailable"
	case 504:
		return "gateway_timeout"
	}
	if class == OutcomeBad {
		return "server_error"
	}
	return "ok"
}

// statusWriter запоминает код ответа. Молчаливый Write означает 200,
// отсутствие и WriteHeader, и Write — отсутствие ответа.
type statusWriter struct {
	http.ResponseWriter
	status int
}

// status возвращает код ответа. Если ни WriteHeader, ни Write не вызывались,
// net/http отдаёт клиенту 200 — значит и в SLI это good, а не «нет ответа».
func (w *statusWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

/**
 * Возвращает Hijacker для protocol switching (RFC 7230 §6.7).
 *
 * Важно: мы не пробиваем Hijacker через вложенные ResponseWriter'ы вроде
 * http.NewResponseController, а отдаём напрямую нижележащий — тот же приём,
 * что и в gorilla/websocket. Иначе WebSocket upgrade в чате вернёт 500.
 */
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}
