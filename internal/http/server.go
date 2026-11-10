package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Nikola4667636/template/internal/config"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
}

func NewServer(cfg *config.Config, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.HTTP_ADDR,
			Handler:           handler,
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
