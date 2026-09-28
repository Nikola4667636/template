package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"template/internal/handler"
	"template/internal/validate"
)

type Server struct {
	httpServer *http.Server
	cfg        *validate.Config
}

func NewServer(cfg *validate.Config, healthHandler *handler.HealthHandler) *Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	httpServer := &http.Server{
		Addr:              cfg.HTTP_ADDR,
		Handler:           r,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		cfg:        cfg,
	}
}

func (s *Server) Run() error {
	shutdownError := make(chan error, 1)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit

		log.Printf("received signal: %s. starting graceful shutdown...", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.SHUTDOWN_TIMEOUT)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			shutdownError <- fmt.Errorf("failed to shutdown http server gracefully: %w", err)
			return
		}

		shutdownError <- nil
	}()

	log.Printf("starting http server on %s", s.cfg.HTTP_ADDR)

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server listen error: %w", err)
	}

	if err := <-shutdownError; err != nil {
		return err
	}

	log.Println("server stopped gracefully")
	return nil
}
