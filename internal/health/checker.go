package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/devraousama-wq/portcrane/internal/config"
	"github.com/devraousama-wq/portcrane/internal/upstream"
)

type Checker struct {
	client   *http.Client
	interval time.Duration
	timeout  time.Duration
	healthy  int
	unhealthy int
	mu       sync.Mutex
}

func NewChecker(cfg config.ActiveHealth) *Checker {
	if !cfg.Enabled {
		return nil
	}
	interval := parseDuration(cfg.Interval, 5*time.Second)
	timeout := parseDuration(cfg.Timeout, 2*time.Second)
	h := cfg.Healthy
	if h <= 0 {
		h = 2
	}
	u := cfg.Unhealthy
	if u <= 0 {
		u = 3
	}
	return &Checker{
		client:    &http.Client{Timeout: timeout},
		interval:  interval,
		timeout:   timeout,
		healthy:   h,
		unhealthy: u,
	}
}

func (c *Checker) Run(ctx context.Context, pool *upstream.Pool, cfg config.ActiveHealth) {
	if c == nil {
		return
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	streak := map[string]int{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, ep := range pool.Snapshot() {
				ok := c.probe(ep, cfg)
				c.mu.Lock()
				if ok {
					streak[ep.ID] = max(streak[ep.ID]+1, 1)
					if streak[ep.ID] >= c.healthy {
						pool.MarkHealthy(ep.ID, true)
					}
				} else {
					streak[ep.ID] = min(streak[ep.ID]-1, -1)
					if -streak[ep.ID] >= c.unhealthy {
						pool.MarkHealthy(ep.ID, false)
					}
				}
				c.mu.Unlock()
			}
		}
	}
}

func (c *Checker) probe(ep upstream.Endpoint, cfg config.ActiveHealth) bool {
	proto := cfg.Protocol
	if proto == "" {
		proto = "http"
	}
	switch proto {
	case "tcp":
		addr := ep.URL.Host
		if ep.URL.Port() == "" {
			addr = net.JoinHostPort(ep.URL.Hostname(), "80")
		}
		conn, err := net.DialTimeout("tcp", addr, c.timeout)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	default:
		path := cfg.Path
		if path == "" {
			path = "/"
		}
		url := fmt.Sprintf("%s://%s%s", ep.URL.Scheme, ep.URL.Host, path)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return false
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode >= 200 && resp.StatusCode < 400
	}
}

type Passive struct {
	maxFailures int
	consecutive int
	mu          sync.Mutex
	failures    map[string]int
	streak      map[string]int
}

func NewPassive(cfg config.PassiveHealth) *Passive {
	if !cfg.Enabled {
		return nil
	}
	maxF := cfg.MaxFailures
	if maxF <= 0 {
		maxF = 5
	}
	cons := cfg.ConsecutiveErrors
	if cons <= 0 {
		cons = 3
	}
	return &Passive{
		maxFailures: maxF,
		consecutive: cons,
		failures:    map[string]int{},
		streak:      map[string]int{},
	}
}

func (p *Passive) Observe(pool *upstream.Pool, id string, status int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if status >= 500 {
		p.streak[id]++
		p.failures[id]++
	} else if status >= 200 && status < 500 {
		p.streak[id] = 0
		if p.failures[id] > 0 {
			p.failures[id]--
		}
	}
	if p.streak[id] >= p.consecutive || p.failures[id] >= p.maxFailures {
		pool.MarkHealthy(id, false)
		p.streak[id] = 0
	} else if status >= 200 && status < 400 {
		pool.MarkHealthy(id, true)
	}
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
