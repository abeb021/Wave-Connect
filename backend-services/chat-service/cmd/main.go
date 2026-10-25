package main

import (
	"chat-service/api/handlers"
	"chat-service/api/middleware"
	"chat-service/api/websocket"
	"chat-service/internal/config"
	"chat-service/internal/repository"
	"chat-service/internal/service"
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

	log := logger.New("chat-service")
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
	hub := websocket.NewHub()
	ws := websocket.NewWSHandler(srv, hub)

	//routing
	r := http.NewServeMux()

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	//r.HandleFunc("POST /api/chat/", h.CreateMessage)
	r.HandleFunc("GET /api/chat/conversation", h.GetConversation)
	r.HandleFunc("GET /api/chat/conversation/{peerID}", h.GetConversationWithPeer)
	r.HandleFunc("GET /api/chat/{id}", h.GetMessage)
	r.HandleFunc("PUT /api/chat/{id}", h.UpdateMessage)
	r.HandleFunc("DELETE /api/chat/{id}", h.DeleteMessage)

	//WebSocket implementation in dev
	r.HandleFunc("GET /api/chat/ws", ws.ServeWS)

	GlobalMiddleware := middleware.Chain(
		pkgmiddleware.Logging(log),
	)

	handler := GlobalMiddleware(r)

	server := http.Server{
		Addr:    ":8082",
		Handler: handler,
	}

	go func() {
		log.Info("server starting", "addr", ":8082")
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