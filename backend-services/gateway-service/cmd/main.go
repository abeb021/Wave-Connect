package main

import (
	handlers "gateway-service/api"
	"gateway-service/api/middleware"
	"gateway-service/internal/config"
	"gateway-service/internal/service"
	"github.com/abeb021/wave-connect/backend-services/pkg/logger"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
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
		middleware.LoggingMiddleware,
	)(mux)

	server := http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	go func() {
		log.Fatal(server.ListenAndServe())
	}()

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down
}
