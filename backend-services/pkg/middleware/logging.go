package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

func LoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w}

		next.ServeHTTP(ww, r)

		lvl := slog.LevelInfo
		if ww.status >= 500 {
			lvl = slog.LevelError
		} else if ww.status >= 400 {
			lvl = slog.LevelWarn
		}
		
		logger.Log(r.Context(), lvl, "http requese",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"remote_addr", r.RemoteAddr,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return LoggingMiddleware(logger, next)
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}