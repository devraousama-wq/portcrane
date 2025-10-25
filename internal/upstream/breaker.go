package upstream

import (
	"sync"
	"time"

	"github.com/devraousama-wq/portcrane/internal/config"
)

type BreakerState int

const (
	BreakerClosed BreakerState = iota
	BreakerOpen
	BreakerHalfOpen
)

type Breaker struct {
	enabled        bool
	failureRatio   float64
	consecutive5xx int
	cooldown       time.Duration
	mu             sync.Mutex
	state          map[string]breakerEntry
}

type breakerEntry struct {
	state       BreakerState
	failures    int
	requests    int
	consecutive int
	openedAt    time.Time
}

func NewBreaker(cfg config.Breaker) *Breaker {
	if !cfg.Enabled {
		return nil
	}
	ratio := cfg.FailureRatio
	if ratio <= 0 {
		ratio = 0.5
	}
	cons := cfg.Consecutive5xx
	if cons <= 0 {
		cons = 5
	}
	cd := parseBreakerDuration(cfg.Cooldown, 30*time.Second)
	return &Breaker{
		enabled:        true,
		failureRatio:   ratio,
		consecutive5xx: cons,
		cooldown:       cd,
		state:          map[string]breakerEntry{},
	}
}

func (b *Breaker) Allow(id string) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	entry := b.state[id]
	switch entry.state {
	case BreakerOpen:
		if time.Since(entry.openedAt) >= b.cooldown {
			entry.state = BreakerHalfOpen
			entry.consecutive = 0
			b.state[id] = entry
			return true
		}
		return false
	default:
		return true
	}
}

func (b *Breaker) Record(id string, status int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	entry := b.state[id]
	entry.requests++
	if status >= 500 {
		entry.failures++
		entry.consecutive++
	} else if status >= 200 {
		entry.consecutive = 0
		if entry.state == BreakerHalfOpen {
			entry.state = BreakerClosed
			entry.failures = 0
			entry.requests = 0
		}
	}
	ratio := 0.0
	if entry.requests > 0 {
		ratio = float64(entry.failures) / float64(entry.requests)
	}
	if entry.consecutive >= b.consecutive5xx || (entry.requests >= 10 && ratio >= b.failureRatio) {
		entry.state = BreakerOpen
		entry.openedAt = time.Now()
	}
	b.state[id] = entry
}

func (b *Breaker) Snapshot() map[string]BreakerState {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]BreakerState, len(b.state))
	for id, entry := range b.state {
		out[id] = entry.state
	}
	return out
}

func parseBreakerDuration(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
