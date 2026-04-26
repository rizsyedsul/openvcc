package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/syedsumx/openvcc/internal/config"
)

// BuildBackendTLSConfig converts a config.BackendTLS into a usable *tls.Config.
// Returns nil if spec is nil.
func BuildBackendTLSConfig(spec *config.BackendTLS) (*tls.Config, error) {
	if spec == nil {
		return nil, nil
	}
	tc := &tls.Config{
		ServerName:         spec.ServerName,
		InsecureSkipVerify: spec.InsecureSkipVerify, //nolint:gosec // explicit opt-in
		MinVersion:         tls.VersionTLS12,
	}
	if spec.CertFile != "" && spec.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(spec.CertFile, spec.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client keypair: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	if spec.CAFile != "" {
		pool, err := loadCAPool(spec.CAFile)
		if err != nil {
			return nil, err
		}
		tc.RootCAs = pool
	}
	return tc, nil
}

func loadCAPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ca file %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("ca file contained no parseable certificates")
	}
	return pool, nil
}
