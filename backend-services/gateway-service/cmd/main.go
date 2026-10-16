package main

import (
	handlers "gateway-service/api"
	"gateway-service/api/middleware"
	"gateway-service/internal/config"
	"gateway-service/internal/service"
	"github.com/abeb021/wave-connect/backend-services/pkg/logger"
	pkgmiddleware "github.com/abeb021/wave-connect/backend-services/pkg/middleware"

	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log := logger.New("gateway-service")

	cfg := config.Load()

	service := service.NewService(
		cfg.AuthServiceURL,
		cfg.ChatServiceURL,
		cfg.FeedServiceURL,
		cfg.ProfileServiceURL,
		cfg.JWTSecret,
	)

	mux := http.NewServeMux()

	handlers.RegisterRoutes(mux, service)

	handler := middleware.Chain(
		middleware.CorsMiddleware,
		pkgmiddleware.Logging(log),
	)(mux)

	server := http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	go func() {
		log.Info("server starting", "addr", ":8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server stopped", "error", err)
		}
	}()

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down
	log.Info("shutting down")
}
