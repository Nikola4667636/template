package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/Nikola4667636/template/internal/config"
)

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

func NewServer(cfg *config.Config, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.HTTP_ADDR,
			Handler:           handler,
			ReadTimeout:       cfg.HTTP_READ_TIMEOUT,
			ReadHeaderTimeout: cfg.HTTP_READ_HEADER_TIMEOUT,
			WriteTimeout:      cfg.HTTP_WRITE_TIMEOUT,
			IdleTimeout:       cfg.HTTP_IDLE_TIMEOUT,
		},
		logger: logger,
	}
}

func (s *Server) Run(ctx context.Context) error {
	// Request cancellation is controlled by net/http; the signal starts graceful draining.
	s.httpServer.BaseContext = func(net.Listener) context.Context { return context.WithoutCancel(ctx) }
	s.logger.InfoContext(ctx, "starting http server", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server listen error: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.InfoContext(ctx, "stopping http server")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		closeErr := s.httpServer.Close()
		return errors.Join(fmt.Errorf("failed to shutdown http server gracefully: %w", err), closeErr)
	}
	s.logger.InfoContext(ctx, "http server stopped gracefully")
	return nil
}
