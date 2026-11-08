package main

import (
	handlers "gateway-service/api"
	"gateway-service/api/middleware"
	"gateway-service/internal/config"
	"gateway-service/internal/service"

	"github.com/abeb021/wave-connect/backend-services/pkg/logger"
	"github.com/abeb021/wave-connect/backend-services/pkg/metrics"
	pkgmiddleware "github.com/abeb021/wave-connect/backend-services/pkg/middleware"

	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := logger.New("gateway-service")

	sloService := flag.String("slo", "gateway", "service layer name for SLI/TBSLO metrics")
	flag.Parse()

	metrics.InitBusinessMetrics()

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

	go func() {
		// Пробер проверяет сам gateway: POST /api/auth/login без тела даёт 400,
		// то есть gateway жив и проксирует auth-service.
		// 4xx считается живым (Classify по 5xx, а не по 2xx/3xx) — иначе gateway
		// навсегда висел бы в probe_up=0 и давал ложный WaveAllReplicasDown.
		if err := metrics.ServeWithProberMethod(":9090", http.MethodPost, *sloService,
			"http://127.0.0.1:8080/api/auth/login", time.Minute, log); err != nil {
			log.Error("metrics server stopped", "error", err)
		}
	}()

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down
	log.Info("shutting down")
}
