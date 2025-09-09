package routing

import (
	"net/http"
	"strings"

	"github.com/devraousama-wq/portcrane/internal/config"
)

func (r *Router) MatchHeaders(req *http.Request) (config.Route, bool) {
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
		if len(route.Match.Headers) > 0 {
			for key, want := range route.Match.Headers {
				got := req.Header.Get(key)
				if got == "" || got != want {
					score = -1
					break
				}
				score += 5
			}
			if score < 0 {
				continue
			}
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
