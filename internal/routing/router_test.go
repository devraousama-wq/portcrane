package routing

import (
	"net/http"
	"testing"

	"github.com/devraousama-wq/portcrane/internal/config"
)

func TestRouterHostAndPath(t *testing.T) {
	r := New([]config.Route{
		{Name: "api", Match: config.RouteMatch{Host: "api.example.com", PathPrefix: "/v1"}, Pool: "api"},
		{Name: "default", Match: config.RouteMatch{Host: "*", PathPrefix: "/"}, Pool: "default"},
	})
	req := httptestRequest("GET", "api.example.com", "/v1/users")
	route, ok := r.Match(req)
	if !ok {
		t.Fatal("expected match")
	}
	if route.Name != "api" {
		t.Fatalf("got route %s", route.Name)
	}
}

func TestRouterWildcardHost(t *testing.T) {
	r := New([]config.Route{
		{Name: "wildcard", Match: config.RouteMatch{Host: "*.example.com", PathPrefix: "/health"}, Pool: "default"},
	})
	req := httptestRequest("GET", "beta.example.com", "/health")
	route, ok := r.Match(req)
	if !ok || route.Name != "wildcard" {
		t.Fatalf("unexpected route %#v ok=%v", route, ok)
	}
}

func TestRouterHeaders(t *testing.T) {
	r := New([]config.Route{
		{
			Name:  "canary",
			Match: config.RouteMatch{Host: "*", PathPrefix: "/", Headers: map[string]string{"X-Canary": "1"}},
			Pool:  "canary",
		},
	})
	req := httptestRequest("GET", "example.com", "/")
	req.Header.Set("X-Canary", "1")
	route, ok := r.MatchHeaders(req)
	if !ok || route.Pool != "canary" {
		t.Fatalf("unexpected route %#v ok=%v", route, ok)
	}
}

func httptestRequest(method, host, path string) *http.Request {
	req, _ := http.NewRequest(method, "http://"+host+path, nil)
	req.Host = host
	return req
}
