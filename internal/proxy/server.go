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
	http   *http.Server
}

func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{cfg: cfg, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	s.http = &http.Server{Handler: mux}
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, len(s.cfg.Listeners))
	started := 0
	for _, listener := range s.cfg.Listeners {
		switch listener.Protocol {
		case "http", "https":
			started++
			go func(l config.Listener) {
				srv := *s.http
				srv.Addr = l.Bind
				s.logger.Info("listener starting", "name", l.Name, "bind", l.Bind, "protocol", l.Protocol)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("listener %s: %w", l.Name, err)
				}
			}(listener)
		default:
			return fmt.Errorf("unsupported listener protocol %q", listener.Protocol)
		}
	}
	if started == 0 {
		return fmt.Errorf("no listeners started")
	}
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout())
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
