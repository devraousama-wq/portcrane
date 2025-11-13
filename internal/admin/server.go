package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/devraousama-wq/portcrane/internal/config"
	"github.com/devraousama-wq/portcrane/internal/upstream"
)

type Server struct {
	token   string
	cfgFn   func() *config.Config
	pools   *upstream.Manager
	auditMu sync.Mutex
	audit   []AuditEntry
}

type AuditEntry struct {
	Action string `json:"action"`
	Detail string `json:"detail"`
}

func New(bind string, token string, cfgFn func() *config.Config, pools *upstream.Manager) *http.Server {
	s := &Server{token: token, cfgFn: cfgFn, pools: pools}
	mux := http.NewServeMux()
	mux.HandleFunc("/config", s.auth(s.handleConfig))
	mux.HandleFunc("/pools", s.auth(s.handlePools))
	mux.HandleFunc("/pools/", s.auth(s.handlePoolByID))
	mux.HandleFunc("/stats", s.auth(s.handleStats))
	return &http.Server{Addr: bind, Handler: mux}
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != s.token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.cfgFn())
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var cfg config.Config
		if err := json.Unmarshal(body, &cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := cfg.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.record("config.apply", "posted new config")
		writeJSON(w, map[string]string{"status": "accepted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	names := s.pools.Names()
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		pool, _ := s.pools.Get(name)
		out = append(out, map[string]any{"name": name, "upstreams": pool.Snapshot()})
	}
	writeJSON(w, out)
}

func (s *Server) handlePoolByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/pools/")
	if id == "" {
		http.Error(w, "missing pool id", http.StatusBadRequest)
		return
	}
	pool, ok := s.pools.Get(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, pool.Snapshot())
	case http.MethodPost:
		if strings.HasSuffix(r.URL.Path, "/drain") {
			for _, ep := range pool.Snapshot() {
				pool.MarkHealthy(ep.ID, false)
			}
			s.record("pool.drain", id)
			writeJSON(w, map[string]string{"status": "draining", "pool": id})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"pools": len(s.pools.Names()),
		"audit": s.auditCopy(),
	})
}

func (s *Server) record(action, detail string) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	s.audit = append(s.audit, AuditEntry{Action: action, Detail: detail})
	if len(s.audit) > 100 {
		s.audit = s.audit[len(s.audit)-100:]
	}
}

func (s *Server) auditCopy() []AuditEntry {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	out := make([]AuditEntry, len(s.audit))
	copy(out, s.audit)
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
