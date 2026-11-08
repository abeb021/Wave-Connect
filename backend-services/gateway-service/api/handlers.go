package handlers

import (
	"gateway-service/api/middleware"
	"gateway-service/internal/service"
	"net/http"

	"github.com/abeb021/wave-connect/backend-services/pkg/metrics"
)

// withSLO считает исход запроса на gateway. Именно здесь видно всё,
// что получил клиент, включая 502 при недоступном downstream-сервисе.
func withSLO(name string, next http.Handler) http.Handler {
	return metrics.Track(name, next)
}

func RegisterRoutes(r *http.ServeMux, srv *service.Service) {
	r.Handle("POST /api/auth/register", withSLO("auth", srv.AuthProxy()))
	r.Handle("POST /api/auth/login", withSLO("auth", srv.AuthProxy()))

	r.Handle("/api/auth/", withSLO("auth",
		middleware.JWTMiddleware(srv.JWTSecret, srv.AuthProxy())))
	r.Handle("/api/feed/", withSLO("feed",
		middleware.JWTMiddleware(srv.JWTSecret, srv.FeedProxy())))
	r.Handle("/api/profile/", withSLO("profile",
		middleware.JWTMiddleware(srv.JWTSecret, srv.ProfileProxy())))
	r.Handle("/api/chat/", withSLO("chat",
		middleware.JWTMiddleware(srv.JWTSecret, srv.ChatProxy())))
}
