package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"profile-service/api/http/handlers"
	"profile-service/api/http/middleware"
	"profile-service/internal/config"
	profilekafka "profile-service/internal/kafka"
	"profile-service/internal/repository"
	"profile-service/internal/service"
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

	log := logger.New("profile-service")

	sloService := flag.String("slo", "profile", "service layer name for SLI/TBSLO metrics")
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

	//backend architecture setup
	repo := repository.NewRepository(dbPool)
	producer, err := profilekafka.NewProducer(cfg.KafkaBroker)
	if err != nil {
		log.Error("kafka producer", "error", err)
		os.Exit(1)
	}
	defer producer.Close()
	srv := service.NewService(repo, producer)
	h := handlers.NewHandler(srv)

	// Kafka consumer
	consumer, err := profilekafka.NewConsumer(cfg.KafkaBroker, "profile-service", repo)
	if err != nil {
		log.Error("kafka consumer", "error", err)
		os.Exit(1)
	}
	defer consumer.Close()

	go func() {
		log.Info("starting kafka consumer")
		if err := consumer.Start(ctx, []string{"user-registered"}); err != nil {
			log.Error("consumer stopped", "error", err)
		}
	}()

	//routing
	api := http.NewServeMux()
	h.RegisterRoutes(api)

	// /health обслуживается отдельно и вне SLI.
	r := http.NewServeMux()
	r.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	r.Handle("/", middleware.Chain(
		func(next http.Handler) http.Handler { return metrics.Track(*sloService, next) },
		pkgmiddleware.Logging(log),
	)(api))

	server := http.Server{
		Addr:    ":8084",
		Handler: r,
	}

	go func() {
		log.Info("server starting", "addr", ":8084")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server stopped", "error", err)
		}
	}()

	go func() {
		log.Info("metrics starting", "addr", ":9090")
		if err := metrics.ServeWithProber(":9090", *sloService, "http://127.0.0.1:8084/health",
			time.Minute, log); err != nil {
			log.Error("metrics server stopped", "error", err)
		}
	}()

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down
	log.Info("shutting down")
}
