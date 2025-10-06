package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Chain struct {
	handlers []func(http.Handler) http.Handler
}

func NewChain(items ...func(http.Handler) http.Handler) Chain {
	return Chain{handlers: items}
}

func (c Chain) Then(next http.Handler) http.Handler {
	for i := len(c.handlers) - 1; i >= 0; i-- {
		next = c.handlers[i](next)
	}
	return next
}

type AuthConfig struct {
	BasicUser   string
	BasicPass   string
	BearerToken string
	HeaderKey   string
	HeaderValue string
}

func Auth(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.BearerToken != "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(auth, "Bearer ") {
					token := strings.TrimPrefix(auth, "Bearer ")
					if subtle.ConstantTimeCompare([]byte(token), []byte(cfg.BearerToken)) == 1 {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			if cfg.HeaderKey != "" && cfg.HeaderValue != "" {
				if r.Header.Get(cfg.HeaderKey) == cfg.HeaderValue {
					next.ServeHTTP(w, r)
					return
				}
			}
			if cfg.BasicUser != "" {
				user, pass, ok := r.BasicAuth()
				if ok &&
					subtle.ConstantTimeCompare([]byte(user), []byte(cfg.BasicUser)) == 1 &&
					subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.BasicPass)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
				w.Header().Set("WWW-Authenticate", `Basic realm="portcrane"`)
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

type RateLimitConfig struct {
	RPS   float64
	Burst int
	KeyFn func(*http.Request) string
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rps     float64
	burst   float64
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	burst := float64(cfg.Burst)
	if burst <= 0 {
		burst = 10
	}
	rps := cfg.RPS
	if rps <= 0 {
		rps = 50
	}
	return &RateLimiter{buckets: map[string]*tokenBucket{}, rps: rps, burst: burst}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	now := time.Now()
	if !ok {
		b = &tokenBucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(rl.burst, b.tokens+elapsed*rl.rps)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func RateLimit(rl *RateLimiter, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	if keyFn == nil {
		keyFn = func(r *http.Request) string { return r.RemoteAddr }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.Allow(keyFn(r)) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type CORSConfig struct {
	AllowOrigin  string
	AllowMethods string
	AllowHeaders string
}

func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := cfg.AllowOrigin
			if origin == "" {
				origin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			if cfg.AllowMethods != "" {
				w.Header().Set("Access-Control-Allow-Methods", cfg.AllowMethods)
			} else {
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			}
			if cfg.AllowHeaders != "" {
				w.Header().Set("Access-Control-Allow-Headers", cfg.AllowHeaders)
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type RewriteConfig struct {
	StripPrefix string
	AddPrefix   string
	SetHeaders  map[string]string
}

func Rewrite(cfg RewriteConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if cfg.StripPrefix != "" && strings.HasPrefix(path, cfg.StripPrefix) {
				path = strings.TrimPrefix(path, cfg.StripPrefix)
				if path == "" {
					path = "/"
				}
			}
			if cfg.AddPrefix != "" {
				path = strings.TrimSuffix(cfg.AddPrefix, "/") + path
			}
			r.URL.Path = path
			for k, v := range cfg.SetHeaders {
				r.Header.Set(k, v)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ParseBearerJWT(token string) (map[string]string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	claims := map[string]string{}
	for _, seg := range strings.Split(string(payload), ",") {
		seg = strings.Trim(seg, "{}\" ")
		kv := strings.SplitN(seg, ":", 2)
		if len(kv) == 2 {
			claims[strings.Trim(kv[0], "\"")] = strings.Trim(kv[1], "\"")
		}
	}
	return claims, len(claims) > 0
}

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format("20060102150405.000")))
		}
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type requestIDKey struct{}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
