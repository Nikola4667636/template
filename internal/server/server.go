package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"template/api"
	"template/internal/handler"
	"template/internal/validate"
)

type Server struct {
	httpServer *http.Server
	cfg        *validate.Config
}

func NewServer(cfg *validate.Config, apiHandler *handler.APIHandler) *Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	api.HandlerWithOptions(apiHandler, api.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: handler.InvalidRequest,
	})

	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.HTTP_ADDR,
			Handler:           r,
			ReadTimeout:       cfg.HTTP_READ_TIMEOUT,
			ReadHeaderTimeout: cfg.HTTP_READ_HEADER_TIMEOUT,
			WriteTimeout:      cfg.HTTP_WRITE_TIMEOUT,
			IdleTimeout:       cfg.HTTP_IDLE_TIMEOUT,
		},
		cfg: cfg,
	}
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		slog.Info("starting http server", "addr", s.httpServer.Addr)

		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server listen error: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutdown signal received, stopping http server", "timeout", s.cfg.SHUTDOWN_TIMEOUT.String())

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.SHUTDOWN_TIMEOUT)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		_ = s.httpServer.Close()
		return fmt.Errorf("failed to shutdown http server gracefully: %w", err)
	}

	if err := <-errCh; err != nil {
		return err
	}

	slog.Info("http server stopped gracefully")
	return nil
}
