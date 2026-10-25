package main

import (
	"auth-service/api/handlers"
	"auth-service/api/middleware"
	"auth-service/internal/config"
	"auth-service/internal/kafka"
	"auth-service/internal/repository"
	"auth-service/internal/service"
	"auth-service/pkg/jwt"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/abeb021/wave-connect/backend-services/pkg/logger"
	"github.com/abeb021/wave-connect/backend-services/pkg/metrics"
	pkgmiddleware "github.com/abeb021/wave-connect/backend-services/pkg/middleware"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	//context and cfg startup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Load()

	log := logger.New("auth-service")
    
	metrics.InitBusinessMetrics()

	//migrations
	m, err := migrate.New("file://migrations", cfg.DatabaseURL())
	if err != nil {
		log.Error("migrate setup", "error", err)
		os.Exit(1)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Error("migrate up", "error", err)
		os.Exit(1)
	}

	//db pool connection
	dbPool, err := repository.NewPool(ctx, cfg.DatabaseURL())
	if err != nil {
		log.Error("db pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	//setup jwt service
	auth := jwt.NewAuthService(cfg.JWTSecret)
	//backend architecture setup
	repo := repository.NewRepository(dbPool)
	producer, err := kafka.NewProducer(cfg.KafkaBroker)
	if err != nil {
		log.Error("kafka producer", "error", err)
		os.Exit(1)
	}
	defer producer.Close()
	srv := service.NewService(repo, auth, producer)
	h := handlers.NewHandler(srv)

	//routing
	r := http.NewServeMux()

	r.HandleFunc("POST /api/auth/register", h.Register)
	r.HandleFunc("POST /api/auth/login", h.Login)
	r.HandleFunc("GET /api/auth/id/", h.GetUserById)
	r.HandleFunc("DELETE /api/auth/", h.DeleteUser)

	GlobalMiddleware := middleware.Chain(
		pkgmiddleware.Logging(log),
	)

	handler := GlobalMiddleware(r)

	server := http.Server{
		Addr:    ":8081",
		Handler: handler,
	}

	go func() {
		log.Info("server starting", "addr", ":8081")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server stopped", "error", err)
		}
	}()

    go func() {
        log.Info("metrics starting", "addr", ":9090")
        if err := metrics.Serve(":9090"); err != nil {
            log.Error("metrics server stopped", "error", err)
        }
    }()

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down
	log.Info("shutting down")
}