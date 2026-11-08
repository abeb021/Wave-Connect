package main

import (
	"context"
	"feed-service/api/handlers"
	"feed-service/api/middleware"
	"feed-service/internal/config"
	feedkafka "feed-service/internal/kafka"
	"feed-service/internal/repository"
	"feed-service/internal/service"
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

	log := logger.New("feed-service")

	sloService := flag.String("slo", "feed", "service layer name for SLI/TBSLO metrics")
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
	srv := service.NewService(repo)
	h := handlers.NewHandler(srv)

	// Kafka consumer
	consumer, err := feedkafka.NewConsumer(cfg.KafkaBroker, "feed-service", repo)
	if err != nil {
		log.Error("kafka consumer", "error", err)
		os.Exit(1)
	}
	defer consumer.Close()

	go func() {
		log.Info("starting kafka consumer")
		if err := consumer.Start(ctx, []string{"profile-updates"}); err != nil {
			log.Error("consumer stopped", "error", err)
		}
	}()

	//routing
	api := http.NewServeMux()

	api.HandleFunc("POST /api/feed/", h.CreatePublication)
	api.HandleFunc("GET /api/feed/", h.GetFeed)
	api.HandleFunc("GET /api/feed/user/{userID}", h.GetPublicationsByUser)
	api.HandleFunc("GET /api/feed/{id}", h.GetPublication)
	api.HandleFunc("PUT /api/feed/{id}", h.UpdatePublication)
	api.HandleFunc("DELETE /api/feed/{id}", h.DeletePublication)

	api.HandleFunc("POST /api/feed/{pubID}/comment/", h.CreateComment)
	api.HandleFunc("GET /api/feed/{pubID}/comment/", h.GetCommentsByPublication)
	api.HandleFunc("DELETE /api/feed/comment/{id}", h.DeleteComment)

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
		Addr:    ":8083",
		Handler: r,
	}

	go func() {
		log.Info("server starting", "addr", ":8083")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server stopped", "error", err)
		}
	}()

	go func() {
		log.Info("metrics starting", "addr", ":9090")
		if err := metrics.ServeWithProber(":9090", *sloService, "http://127.0.0.1:8083/health",
			time.Minute, log); err != nil {
			log.Error("metrics server stopped", "error", err)
		}
	}()

	down := make(chan os.Signal, 1)
	signal.Notify(down, syscall.SIGTERM, syscall.SIGINT)
	<-down
	log.Info("shutting down")
}
