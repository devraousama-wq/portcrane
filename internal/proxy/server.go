package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/devraousama-wq/portcrane/internal/config"
)

type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	http   []*http.Server
}

func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{cfg: cfg, logger: logger}, nil
}

func NewWithPath(path string, cfg *config.Config, logger *slog.Logger) (*Server, error) {
	return New(cfg, logger)
}

func (s *Server) Run(ctx context.Context) error {
	handler := NewHTTPProxy(s.cfg, s.logger)
	errCh := make(chan error, 4)
	for _, listener := range s.cfg.Listeners {
		if listener.Protocol != "http" {
			continue
		}
		srv := &http.Server{Addr: listener.Bind, Handler: handler}
		s.http = append(s.http, srv)
		go func(l config.Listener, srv *http.Server) {
			s.logger.Info("listener starting", "name", l.Name, "bind", l.Bind)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("listener %s: %w", l.Name, err)
			}
		}(listener, srv)
	}
	select {
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout())
		defer cancel()
		for _, srv := range s.http {
			_ = srv.Shutdown(shCtx)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
