package routing

import (
	"net/http"
	"strings"

	"github.com/devraousama-wq/portcrane/internal/config"
)

type Match struct {
	Route config.Route
	Score int
}

type Router struct {
	routes []config.Route
}

func New(routes []config.Route) *Router {
	cp := make([]config.Route, len(routes))
	copy(cp, routes)
	return &Router{routes: cp}
}

func (r *Router) Match(req *http.Request) (config.Route, bool) {
	host := stripPort(req.Host)
	path := req.URL.Path
	method := req.Method
	var best Match
	found := false
	for _, route := range r.routes {
		score := 0
		if route.Match.Host != "" && route.Match.Host != "*" {
			if !hostMatch(host, route.Match.Host) {
				continue
			}
			score += 10
		}
		if route.Match.PathPrefix != "" {
			if !strings.HasPrefix(path, route.Match.PathPrefix) {
				continue
			}
			score += len(route.Match.PathPrefix)
		}
		if route.Match.Method != "" && !strings.EqualFold(route.Match.Method, method) {
			continue
		}
		if score >= best.Score {
			best = Match{Route: route, Score: score}
			found = true
		}
	}
	if !found {
		return config.Route{}, false
	}
	return best.Route, true
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 && strings.Count(host, ":") == 1 {
		return host[:i]
	}
	if strings.HasPrefix(host, "[") {
		if i := strings.LastIndex(host, "]:"); i >= 0 {
			return host[:i+1]
		}
	}
	return host
}

func hostMatch(host, pattern string) bool {
	if strings.EqualFold(host, pattern) {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		return strings.HasSuffix(strings.ToLower(host), strings.ToLower(suffix))
	}
	return false
}
