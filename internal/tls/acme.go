package tlsmgr

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/devraousama-wq/portcrane/internal/config"
	"golang.org/x/crypto/acme/autocert"
)

type ACMEManager struct {
	manager *autocert.Manager
	hsts    bool
}

func NewACME(cfg config.ACME) (*ACMEManager, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.Email == "" {
		return nil, fmt.Errorf("acme email required")
	}
	cache := cfg.CacheDir
	if cache == "" {
		cache = "data/acme"
	}
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.Hosts...),
		Cache:      autocert.DirCache(cache),
		Email:      cfg.Email,
	}
	return &ACMEManager{manager: m, hsts: true}, nil
}

func (a *ACMEManager) TLSConfig() *tls.Config {
	if a == nil {
		return nil
	}
	return a.manager.TLSConfig()
}

func (a *ACMEManager) ChallengeHandler() http.Handler {
	if a == nil {
		return http.NotFoundHandler()
	}
	return a.manager.HTTPHandler(nil)
}

func (a *ACMEManager) HSTS() bool {
	if a == nil {
		return false
	}
	return a.hsts
}

func (a *ACMEManager) Run(ctx context.Context) error {
	if a == nil {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}
