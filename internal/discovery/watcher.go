package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/devraousama-wq/portcrane/internal/config"
	"github.com/devraousama-wq/portcrane/internal/upstream"
)

type Event struct {
	Pool      string
	Upstreams []config.Upstream
}

type Bus struct {
	mu       sync.RWMutex
	handlers []func(Event)
}

func NewBus() *Bus {
	return &Bus{}
}

func (b *Bus) Subscribe(fn func(Event)) {
	b.mu.Lock()
	b.handlers = append(b.handlers, fn)
	b.mu.Unlock()
}

func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, fn := range b.handlers {
		fn(ev)
	}
}

type FileWatcher struct {
	path     string
	interval time.Duration
	bus      *Bus
	pool     string
}

func NewFileWatcher(cfg config.FileDiscovery, pool string, bus *Bus) *FileWatcher {
	if !cfg.Enabled {
		return nil
	}
	interval := 10 * time.Second
	if cfg.Interval != "" {
		if d, err := time.ParseDuration(cfg.Interval); err == nil {
			interval = d
		}
	}
	return &FileWatcher{path: cfg.Path, interval: interval, bus: bus, pool: pool}
}

func (f *FileWatcher) Run(ctx context.Context) error {
	if f == nil {
		return nil
	}
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	var last []byte
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			data, err := os.ReadFile(f.path)
			if err != nil {
				continue
			}
			if string(data) == string(last) {
				continue
			}
			last = data
			var ups []config.Upstream
			if err := json.Unmarshal(data, &ups); err != nil {
				continue
			}
			f.bus.Publish(Event{Pool: f.pool, Upstreams: ups})
		}
	}
}

type DNSWatcher struct {
	name     string
	interval time.Duration
	bus      *Bus
	pool     string
}

func NewDNSWatcher(cfg config.DNSDiscovery, pool string, bus *Bus) *DNSWatcher {
	if !cfg.Enabled {
		return nil
	}
	interval := 30 * time.Second
	if cfg.Interval != "" {
		if d, err := time.ParseDuration(cfg.Interval); err == nil {
			interval = d
		}
	}
	return &DNSWatcher{name: cfg.Name, interval: interval, bus: bus, pool: pool}
}

func (d *DNSWatcher) Run(ctx context.Context) error {
	if d == nil {
		return nil
	}
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			addrs, err := lookupHost(ctx, d.name)
			if err != nil || len(addrs) == 0 {
				continue
			}
			ups := make([]config.Upstream, 0, len(addrs))
			for i, addr := range addrs {
				ups = append(ups, config.Upstream{
					ID:      fmt.Sprintf("dns-%d", i),
					Address: fmt.Sprintf("http://%s", addr),
					Weight:  1,
				})
			}
			d.bus.Publish(Event{Pool: d.pool, Upstreams: ups})
		}
	}
}

func ApplyEvent(mgr *upstream.Manager, ev Event, pools config.Pools) error {
	poolCfg, ok := pools[ev.Pool]
	if !ok {
		return fmt.Errorf("unknown pool %s", ev.Pool)
	}
	poolCfg.Upstreams = ev.Upstreams
	pools[ev.Pool] = poolCfg
	return mgr.Replace(pools)
}
