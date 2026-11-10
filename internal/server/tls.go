package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureSelfSignedCertificate ensures that a valid TLS certificate and private key exist in
// certDir. If valid files do not exist or have expired, a new ECDSA P-256 keypair and self-signed
// X.509 certificate are generated with SANs covering localhost and provided IPs/hostnames.
func EnsureSelfSignedCertificate(certDir string, hostnames []string) (string, string, error) {
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return "", "", fmt.Errorf("failed to create certificate directory: %w", err)
	}

	certPath := filepath.Join(certDir, "cert.pem")
	keyPath := filepath.Join(certDir, "key.pem")

	// Check if existing certificate is valid, keypair matches, and all requested hostnames/IPs are covered
	if pair, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil && len(pair.Certificate) > 0 {
		if parsedCert, err := x509.ParseCertificate(pair.Certificate[0]); err == nil {
			// Ensure certificate has at least 7 days before expiry
			if time.Until(parsedCert.NotAfter) > 7*24*time.Hour {
				missingHost := false
				for _, h := range hostnames {
					if h == "" || h == "localhost" || h == "127.0.0.1" {
						continue
					}
					if ip := net.ParseIP(h); ip != nil {
						hasIP := false
						for _, certIP := range parsedCert.IPAddresses {
							if certIP.Equal(ip) {
								hasIP = true
								break
							}
						}
						if !hasIP {
							missingHost = true
							break
						}
					} else {
						hasDNS := false
						for _, dns := range parsedCert.DNSNames {
							if strings.EqualFold(dns, h) {
								hasDNS = true
								break
							}
						}
						if !hasDNS {
							missingHost = true
							break
						}
					}
				}
				if !missingHost {
					return certPath, keyPath, nil
				}
			}
		}
	}

	// Generate new ECDSA P-256 private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate ECDSA private key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate serial number: %w", err)
	}

	dnsNames := []string{"localhost"}
	ipAddresses := []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback}

	for _, h := range hostnames {
		if h == "" || h == "localhost" || h == "127.0.0.1" {
			continue
		}
		if ip := net.ParseIP(h); ip != nil {
			ipAddresses = append(ipAddresses, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "SqlFox Local & LAN Server",
			Organization: []string{"SqlFox"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour), // mitigate clock skew
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write cert.pem
	certOut, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", "", fmt.Errorf("failed to write certificate file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return "", "", fmt.Errorf("failed to PEM encode certificate: %w", err)
	}

	// Write key.pem
	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key: %w", err)
	}

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", "", fmt.Errorf("failed to write key file: %w", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return "", "", fmt.Errorf("failed to PEM encode private key: %w", err)
	}

	return certPath, keyPath, nil
}

// LoadOrCreateTLSConfig creates a modern, secure TLS configuration using custom certificate/key
// paths if provided, or automatically generating/loading a self-signed keypair from certDir.
func LoadOrCreateTLSConfig(certFile, keyFile, certDir string, hostnames []string) (*tls.Config, string, string, error) {
	if (certFile != "" && keyFile == "") || (certFile == "" && keyFile != "") {
		return nil, "", "", fmt.Errorf("both certificate and private key files must be specified")
	}

	var finalCert, finalKey string
	if certFile != "" && keyFile != "" {
		finalCert = certFile
		finalKey = keyFile
	} else {
		c, k, err := EnsureSelfSignedCertificate(certDir, hostnames)
		if err != nil {
			return nil, "", "", err
		}
		finalCert = c
		finalKey = k
	}

	pair, err := tls.LoadX509KeyPair(finalCert, finalKey)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to load TLS key pair: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{pair},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	}

	return cfg, finalCert, finalKey, nil
}
