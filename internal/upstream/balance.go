package upstream

import (
	"hash/fnv"
	"math"
	"sync/atomic"
)

type Selector interface {
	Next(endpoints []*Endpoint, clientIP string) (*Endpoint, error)
}

func NewSelector(policy string, endpoints []*Endpoint) Selector {
	switch policy {
	case "weighted_round_robin":
		return &WeightedRoundRobin{}
	case "least_connections":
		return &LeastConnections{}
	case "ip_hash":
		return &IPHash{}
	case "ewma":
		return NewEWMA(endpoints)
	default:
		return &RoundRobin{}
	}
}

type RoundRobin struct {
	idx uint64
}

func (r *RoundRobin) Next(endpoints []*Endpoint, _ string) (*Endpoint, error) {
	healthy := filterHealthy(endpoints)
	if len(healthy) == 0 {
		return nil, ErrNoHealthyUpstream
	}
	i := atomic.AddUint64(&r.idx, 1) - 1
	return healthy[int(i)%len(healthy)], nil
}

type WeightedRoundRobin struct {
	idx uint64
}

func (w *WeightedRoundRobin) Next(endpoints []*Endpoint, _ string) (*Endpoint, error) {
	healthy := filterHealthy(endpoints)
	if len(healthy) == 0 {
		return nil, ErrNoHealthyUpstream
	}
	total := 0
	for _, ep := range healthy {
		total += ep.Weight
	}
	if total == 0 {
		return healthy[0], nil
	}
	n := int(atomic.AddUint64(&w.idx, 1)-1) % total
	acc := 0
	for _, ep := range healthy {
		acc += ep.Weight
		if n < acc {
			return ep, nil
		}
	}
	return healthy[len(healthy)-1], nil
}

type LeastConnections struct{}

func (l *LeastConnections) Next(endpoints []*Endpoint, _ string) (*Endpoint, error) {
	healthy := filterHealthy(endpoints)
	if len(healthy) == 0 {
		return nil, ErrNoHealthyUpstream
	}
	var pick *Endpoint
	min := math.MaxInt32
	for _, ep := range healthy {
		if ep.MaxConn > 0 && ep.Active >= ep.MaxConn {
			continue
		}
		if ep.Active < min {
			min = ep.Active
			pick = ep
		}
	}
	if pick == nil {
		return nil, ErrNoHealthyUpstream
	}
	return pick, nil
}

type IPHash struct{}

func (h *IPHash) Next(endpoints []*Endpoint, clientIP string) (*Endpoint, error) {
	healthy := filterHealthy(endpoints)
	if len(healthy) == 0 {
		return nil, ErrNoHealthyUpstream
	}
	if clientIP == "" {
		return healthy[0], nil
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(clientIP))
	idx := hasher.Sum32() % uint32(len(healthy))
	return healthy[idx], nil
}

type EWMA struct {
	latency map[string]float64
	alpha   float64
}

func NewEWMA(endpoints []*Endpoint) *EWMA {
	lat := make(map[string]float64, len(endpoints))
	for _, ep := range endpoints {
		lat[ep.ID] = 100
	}
	return &EWMA{latency: lat, alpha: 0.2}
}

func (e *EWMA) Next(endpoints []*Endpoint, _ string) (*Endpoint, error) {
	healthy := filterHealthy(endpoints)
	if len(healthy) == 0 {
		return nil, ErrNoHealthyUpstream
	}
	var pick *Endpoint
	best := math.MaxFloat64
	for _, ep := range healthy {
		lat := e.latency[ep.ID]
		if lat == 0 {
			lat = 100
		}
		if lat < best {
			best = lat
			pick = ep
		}
	}
	return pick, nil
}

func (e *EWMA) Observe(id string, ms float64) {
	if ms <= 0 {
		return
	}
	prev := e.latency[id]
	if prev == 0 {
		prev = ms
	}
	e.latency[id] = e.alpha*ms + (1-e.alpha)*prev
}

func filterHealthy(endpoints []*Endpoint) []*Endpoint {
	out := make([]*Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Healthy {
			out = append(out, ep)
		}
	}
	return out
}
