package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

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
	route, ok := p.router.MatchHeaders(r)
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
	upstream, err := net.DialTimeout("tcp", t.target, 5*time.Second)
	if err != nil {
		t.logger.Error("tcp dial", "target", t.target, "error", err)
		return
	}
	defer upstream.Close()
	go ioCopy(upstream, client)
	ioCopy(client, upstream)
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
