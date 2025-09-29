package tlsmgr

import (
	"crypto/tls"
	"fmt"
	"os"

	"github.com/devraousama-wq/portcrane/internal/config"
)

type Manager struct {
	certs map[string]tls.Certificate
	hsts  bool
}

func NewStatic(cfg config.TLS) (*Manager, error) {
	m := &Manager{hsts: cfg.HSTS, certs: map[string]tls.Certificate{}}
	for _, item := range cfg.Certs {
		cert, err := tls.LoadX509KeyPair(item.CertFile, item.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load cert %s: %w", item.Name, err)
		}
		for _, host := range item.Hosts {
			m.certs[host] = cert
		}
	}
	return m, nil
}

func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if cert, ok := m.certs[info.ServerName]; ok {
				return &cert, nil
			}
			for host, cert := range m.certs {
				if host == info.ServerName {
					return &cert, nil
				}
			}
			return nil, fmt.Errorf("no certificate for %s", info.ServerName)
		},
	}
}

func (m *Manager) HSTS() bool {
	return m.hsts
}

func CertExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
