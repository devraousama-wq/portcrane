package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/devraousama-wq/portcrane/internal/config"
	"github.com/devraousama-wq/portcrane/internal/routing"
)

type HTTPProxy struct {
	cfg    *config.Config
	router *routing.Router
	logger *slog.Logger
}

func NewHTTPProxy(cfg *config.Config, logger *slog.Logger) *HTTPProxy {
	return &HTTPProxy{cfg: cfg, router: routing.New(cfg.Routes), logger: logger}
}

func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := p.router.Match(r)
	if !ok {
		http.Error(w, "no route", http.StatusNotFound)
		return
	}
	pool, ok := p.cfg.Pools[route.Pool]
	if !ok || len(pool.Upstreams) == 0 {
		http.Error(w, "unknown pool", http.StatusBadGateway)
		return
	}
	targetURL, err := url.Parse(pool.Upstreams[0].Address)
	if err != nil {
		http.Error(w, "bad upstream", http.StatusBadGateway)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(w, r)
}
