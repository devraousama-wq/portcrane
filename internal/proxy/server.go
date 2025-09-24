package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/devraousama-wq/portcrane/internal/config"
	"github.com/devraousama-wq/portcrane/internal/health"
	"github.com/devraousama-wq/portcrane/internal/upstream"
)

type Server struct {
	cfgPath string
	store   *config.Store
	cfg     *config.Config
	logger  *slog.Logger
	pools   *upstream.Manager
	http    []*http.Server
	tcp     []*TCPProxy
	wg      sync.WaitGroup
}

func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	pools, err := upstream.NewManager(cfg.Pools)
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, logger: logger, pools: pools, store: config.NewStore("", cfg, logger)}, nil
}

func NewWithPath(path string, cfg *config.Config, logger *slog.Logger) (*Server, error) {
	s, err := New(cfg, logger)
	if err != nil {
		return nil, err
	}
	s.cfgPath = path
	s.store = config.NewStore(path, cfg, logger)
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	cfg := s.cfg
	if s.store != nil {
		cfg = s.store.Current()
	}
	handler := NewHTTPProxy(cfg, s.pools, s.logger)
	for _, poolName := range s.pools.Names() {
		pool, _ := s.pools.Get(poolName)
		active := cfg.Pools[poolName].Health.Active
		checker := health.NewChecker(active)
		if checker != nil {
			s.wg.Add(1)
			go func(p *upstream.Pool, active config.ActiveHealth) {
				defer s.wg.Done()
				checker.Run(ctx, p, active)
			}(pool, active)
		}
	}
	if s.cfgPath != "" {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_ = s.store.Watch(ctx, func(next *config.Config) {
				if err := s.pools.Replace(next.Pools); err != nil {
					s.logger.Warn("pool reload failed", "error", err)
				}
			})
		}()
	}
	errCh := make(chan error, 8)
	for _, listener := range cfg.Listeners {
		switch listener.Protocol {
		case "http":
			srv := &http.Server{Addr: listener.Bind, Handler: handler}
			s.http = append(s.http, srv)
			s.wg.Add(1)
			go func(l config.Listener, srv *http.Server) {
				defer s.wg.Done()
				s.logger.Info("listener starting", "name", l.Name, "bind", l.Bind, "protocol", l.Protocol)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("listener %s: %w", l.Name, err)
				}
			}(listener, srv)
		case "tcp":
			target := listener.TLS
			if target == "" {
				return fmt.Errorf("tcp listener %s requires target in tls field", listener.Name)
			}
			tcp := NewTCPProxy(listener.Bind, target, s.logger)
			s.tcp = append(s.tcp, tcp)
			s.wg.Add(1)
			go func(p *TCPProxy) {
				defer s.wg.Done()
				if err := p.Run(); err != nil {
					errCh <- err
				}
			}(tcp)
		}
	}
	select {
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
		defer cancel()
		for _, srv := range s.http {
			_ = srv.Shutdown(shCtx)
		}
		s.wg.Wait()
		return nil
	case err := <-errCh:
		return err
	}
}
