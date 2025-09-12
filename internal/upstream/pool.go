package upstream

import (
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/devraousama-wq/portcrane/internal/config"
)

var ErrNoHealthyUpstream = errors.New("no healthy upstream available")

type Endpoint struct {
	ID      string
	Address string
	Weight  int
	MaxConn int
	URL     *url.URL
	Healthy bool
	Active  int
}

type Pool struct {
	name      string
	policy    string
	endpoints []*Endpoint
	mu        sync.RWMutex
	selector  Selector
}

func NewPool(name string, cfg config.Pool) (*Pool, error) {
	eps := make([]*Endpoint, 0, len(cfg.Upstreams))
	for _, u := range cfg.Upstreams {
		parsed, err := url.Parse(u.Address)
		if err != nil {
			return nil, fmt.Errorf("upstream %s: %w", u.ID, err)
		}
		weight := u.Weight
		if weight <= 0 {
			weight = 1
		}
		eps = append(eps, &Endpoint{
			ID:      u.ID,
			Address: u.Address,
			Weight:  weight,
			MaxConn: u.MaxConns,
			URL:     parsed,
			Healthy: true,
		})
	}
	p := &Pool{name: name, policy: cfg.Policy, endpoints: eps}
	p.selector = NewSelector(cfg.Policy, eps)
	return p, nil
}

func (p *Pool) Name() string {
	return p.name
}

func (p *Pool) Pick(clientIP string) (*Endpoint, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.selector.Next(p.endpoints, clientIP)
}

func (p *Pool) MarkHealthy(id string, healthy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, ep := range p.endpoints {
		if ep.ID == id {
			ep.Healthy = healthy
			return
		}
	}
}

func (p *Pool) Snapshot() []Endpoint {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Endpoint, len(p.endpoints))
	for i, ep := range p.endpoints {
		out[i] = *ep
	}
	return out
}

type Manager struct {
	pools map[string]*Pool
	mu    sync.RWMutex
}

func NewManager(cfg config.Pools) (*Manager, error) {
	m := &Manager{pools: make(map[string]*Pool, len(cfg))}
	for name, poolCfg := range cfg {
		p, err := NewPool(name, poolCfg)
		if err != nil {
			return nil, err
		}
		m.pools[name] = p
	}
	return m, nil
}

func (m *Manager) Get(name string) (*Pool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.pools[name]
	return p, ok
}

func (m *Manager) Replace(cfg config.Pools) error {
	next, err := NewManager(cfg)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.pools = next.pools
	m.mu.Unlock()
	return nil
}

func (m *Manager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.pools))
	for name := range m.pools {
		out = append(out, name)
	}
	return out
}
