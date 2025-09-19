package health

import (
	"testing"

	"github.com/devraousama-wq/portcrane/internal/config"
	"github.com/devraousama-wq/portcrane/internal/upstream"
)

func TestPassiveEjects(t *testing.T) {
	pool, err := upstream.NewPool("default", config.Pool{
		Policy: "round_robin",
		Upstreams: []config.Upstream{
			{ID: "a", Address: "http://127.0.0.1:1", Weight: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := NewPassive(config.PassiveHealth{Enabled: true, ConsecutiveErrors: 2})
	p.Observe(pool, "a", 500)
	p.Observe(pool, "a", 500)
	snap := pool.Snapshot()
	if snap[0].Healthy {
		t.Fatal("expected upstream ejected")
	}
}

func TestActiveCheckerConstruct(t *testing.T) {
	c := NewChecker(config.ActiveHealth{
		Enabled:  true,
		Protocol: "http",
		Path:     "/health",
		Interval: "5s",
		Timeout:  "1s",
	})
	if c == nil {
		t.Fatal("expected checker")
	}
}
