package tls

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"
	"os"
)

type Config struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	MinVersion string
	MaxVersion string
}

func DefaultConfig() Config {
	return Config{
		MinVersion: "TLS1.2",
		MaxVersion: "TLS1.3",
	}
}

func LoadTLSConfig(config Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   parseTLSVersion(config.MinVersion),
		MaxVersion:   parseTLSVersion(config.MaxVersion),
	}

	if config.CAFile != "" {
		caCert, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, err
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	}

	return tlsConfig, nil
}

func parseTLSVersion(version string) uint16 {
	switch version {
	case "TLS1.0":
		return tls.VersionTLS10
	case "TLS1.1":
		return tls.VersionTLS11
	case "TLS1.2":
		return tls.VersionTLS12
	case "TLS1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

func CreateServer(addr string, handler http.Handler, tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: tlsConfig,
	}
}

func ListenAndServe(server *http.Server, tlsConfig *tls.Config) error {
	if tlsConfig != nil {
		log.Println("Starting server with TLS")
		return server.ListenAndServeTLS("", "")
	}
	log.Println("Starting server without TLS")
	return server.ListenAndServe()
}
