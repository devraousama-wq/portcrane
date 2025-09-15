package upstream

import (
	"testing"

	"github.com/devraousama-wq/portcrane/internal/config"
)

func TestRoundRobin(t *testing.T) {
	eps := []*Endpoint{
		{ID: "a", Healthy: true},
		{ID: "b", Healthy: true},
	}
	sel := &RoundRobin{}
	first, err := sel.Next(eps, "")
	if err != nil || first.ID != "a" {
		t.Fatalf("first pick = %#v err=%v", first, err)
	}
	second, err := sel.Next(eps, "")
	if err != nil || second.ID != "b" {
		t.Fatalf("second pick = %#v err=%v", second, err)
	}
}

func TestWeightedRoundRobin(t *testing.T) {
	eps := []*Endpoint{
		{ID: "heavy", Weight: 3, Healthy: true},
		{ID: "light", Weight: 1, Healthy: true},
	}
	sel := &WeightedRoundRobin{}
	counts := map[string]int{}
	for i := 0; i < 8; i++ {
		ep, err := sel.Next(eps, "")
		if err != nil {
			t.Fatal(err)
		}
		counts[ep.ID]++
	}
	if counts["heavy"] < counts["light"] {
		t.Fatalf("expected heavier weight to win more often: %#v", counts)
	}
}

func TestIPHashSticky(t *testing.T) {
	eps := []*Endpoint{
		{ID: "a", Healthy: true},
		{ID: "b", Healthy: true},
	}
	sel := &IPHash{}
	first, _ := sel.Next(eps, "203.0.113.10")
	second, _ := sel.Next(eps, "203.0.113.10")
	if first.ID != second.ID {
		t.Fatalf("expected sticky selection, got %s then %s", first.ID, second.ID)
	}
}

func TestPoolManager(t *testing.T) {
	mgr, err := NewManager(config.Pools{
		"default": {
			Policy: "round_robin",
			Upstreams: []config.Upstream{
				{ID: "one", Address: "http://127.0.0.1:9001", Weight: 1},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pool, ok := mgr.Get("default")
	if !ok {
		t.Fatal("missing pool")
	}
	ep, err := pool.Pick("")
	if err != nil || ep.ID != "one" {
		t.Fatalf("pick = %#v err=%v", ep, err)
	}
}

func TestBreakerOpens(t *testing.T) {
	b := NewBreaker(config.Breaker{Enabled: true, Consecutive5xx: 2})
	if !b.Allow("u1") {
		t.Fatal("expected closed breaker")
	}
	b.Record("u1", 500)
	b.Record("u1", 500)
	if b.Allow("u1") {
		t.Fatal("expected open breaker")
	}
}
