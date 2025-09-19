package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/devraousama-wq/portcrane/internal/config"
	"github.com/devraousama-wq/portcrane/internal/health"
	"github.com/devraousama-wq/portcrane/internal/routing"
	"github.com/devraousama-wq/portcrane/internal/upstream"
)

type HTTPProxy struct {
	router  *routing.Router
	pools   *upstream.Manager
	passive *health.Passive
	breaker *upstream.Breaker
	logger  *slog.Logger
	ewma    *upstream.EWMA
}

func NewHTTPProxy(cfg *config.Config, pools *upstream.Manager, logger *slog.Logger) *HTTPProxy {
	router := routing.New(cfg.Routes)
	var ewma *upstream.EWMA
	for _, poolCfg := range cfg.Pools {
		if poolCfg.Policy == "ewma" {
			eps := make([]*upstream.Endpoint, 0)
			for _, u := range poolCfg.Upstreams {
				parsed, _ := url.Parse(u.Address)
				eps = append(eps, &upstream.Endpoint{ID: u.ID, URL: parsed})
			}
			ewma = upstream.NewEWMA(eps)
			break
		}
	}
	var breaker *upstream.Breaker
	for _, poolCfg := range cfg.Pools {
		if poolCfg.Breaker.Enabled {
			breaker = upstream.NewBreaker(poolCfg.Breaker)
			break
		}
	}
	return &HTTPProxy{
		router:  router,
		pools:   pools,
		passive: health.NewPassive(firstPassive(cfg.Pools)),
		breaker: breaker,
		logger:  logger,
		ewma:    ewma,
	}
}

func firstPassive(pools config.Pools) config.PassiveHealth {
	for _, p := range pools {
		return p.Health.Passive
	}
	return config.PassiveHealth{}
}

func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := p.router.MatchHeaders(r)
	if !ok {
		http.Error(w, "no route", http.StatusNotFound)
		return
	}
	pool, ok := p.pools.Get(route.Pool)
	if !ok {
		http.Error(w, "unknown pool", http.StatusBadGateway)
		return
	}
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	ep, err := pool.Pick(clientIP)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if p.breaker != nil && !p.breaker.Allow(ep.ID) {
		http.Error(w, "circuit open", http.StatusServiceUnavailable)
		return
	}
	start := time.Now()
	target := *ep.URL
	target.Path = singleJoin(target.Path, r.URL.Path)
	target.RawQuery = r.URL.RawQuery
	proxy := httputil.NewSingleHostReverseProxy(&target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		p.logger.Error("proxy error", "upstream", ep.ID, "error", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if p.passive != nil {
			p.passive.Observe(pool, ep.ID, resp.StatusCode)
		}
		if p.breaker != nil {
			p.breaker.Record(ep.ID, resp.StatusCode)
		}
		if p.ewma != nil {
			p.ewma.Observe(ep.ID, float64(time.Since(start).Milliseconds()))
		}
		return nil
	}
	chain := proxy
	chain.ServeHTTP(w, r)
}

func singleJoin(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	as := strings.HasSuffix(a, "/")
	bs := strings.HasPrefix(b, "/")
	switch {
	case as && bs:
		return a + b[1:]
	case !as && !bs:
		return a + "/" + b
	default:
		return a + b
	}
}

type TCPProxy struct {
	listen string
	target string
	logger *slog.Logger
}

func NewTCPProxy(listen, target string, logger *slog.Logger) *TCPProxy {
	return &TCPProxy{listen: listen, target: target, logger: logger}
}

func (t *TCPProxy) Run() error {
	ln, err := net.Listen("tcp", t.listen)
	if err != nil {
		return err
	}
	for {
		client, err := ln.Accept()
		if err != nil {
			return err
		}
		go t.handle(client)
	}
}

func (t *TCPProxy) handle(client net.Conn) {
	defer client.Close()
	upstreamConn, err := net.DialTimeout("tcp", t.target, 5*time.Second)
	if err != nil {
		t.logger.Error("tcp dial", "target", t.target, "error", err)
		return
	}
	defer upstreamConn.Close()
	go ioCopy(upstreamConn, client)
	ioCopy(client, upstreamConn)
}

func ioCopy(dst net.Conn, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
