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
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	sloService := flag.String("slo", "auth", "service layer name for SLI/TBSLO metrics")
	flag.Parse()

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
	api := http.NewServeMux()
	api.HandleFunc("POST /api/auth/register", h.Register)
	api.HandleFunc("POST /api/auth/login", h.Login)
	api.HandleFunc("GET /api/auth/id/", h.GetUserById)
	api.HandleFunc("DELETE /api/auth/", h.DeleteUser)

	// /health обслуживается отдельно и вне SLI: проверка живости не должна
	// портить метрику доступности бизнес-трафика.
	r := http.NewServeMux()
	r.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	r.Handle("/", middleware.Chain(
		func(next http.Handler) http.Handler { return metrics.Track(*sloService, next) },
		pkgmiddleware.Logging(log),
	)(api))

	server := http.Server{
		Addr:    ":8081",
		Handler: r,
	}

	go func() {
		log.Info("server starting", "addr", ":8081")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server stopped", "error", err)
		}
	}()

	go func() {
		log.Info("metrics starting", "addr", ":9090")
		if err := metrics.ServeWithProber(":9090", *sloService, "http://127.0.0.1:8081/health",
			time.Minute, log); err != nil {
			log.Error("metrics server stopped", "error", err)
		}
	}()

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down
	log.Info("shutting down")
}
