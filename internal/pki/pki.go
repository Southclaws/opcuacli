// Package pki generates and inspects the client key pair an OPC UA secure
// channel needs. Any policy other than None requires the client to present a
// certificate, and the specification requires that certificate to carry the
// application URI in a subject alternative name, which is the detail that most
// often makes a hand-rolled certificate rejected.
package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// KeyPair is a generated certificate and its private key, in both PEM form (for
// writing to disk) and DER form (for handing straight to a client).
type KeyPair struct {
	CertPEM  []byte
	KeyPEM   []byte
	CertDER  []byte
	CertPath string
	KeyPath  string
}

// Request describes the certificate to generate.
type Request struct {
	CommonName     string
	ApplicationURI string
	DNSNames       []string
	IPAddresses    []string
	Bits           int
	Validity       time.Duration
}

// DefaultApplicationURI is the URI this client announces, and the URI embedded
// in a generated certificate. A server ties a trusted certificate to the
// application URI in it, so the two must agree.
func DefaultApplicationURI() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "localhost"
	}
	return "urn:" + host + ":opcuacli"
}

// Generate creates a self-signed client certificate and RSA key.
func Generate(request Request) (*KeyPair, error) {
	if request.Bits == 0 {
		request.Bits = 2048
	}
	if request.Bits < 2048 {
		// Every security policy still in use requires at least 2048 bits; a
		// shorter key produces a certificate every server will refuse.
		return nil, fmt.Errorf("key size %d is too small: OPC UA security policies require at least 2048 bits", request.Bits)
	}
	if request.Validity == 0 {
		request.Validity = 10 * 365 * 24 * time.Hour
	}
	if request.CommonName == "" {
		request.CommonName = "opcuacli"
	}
	if request.ApplicationURI == "" {
		request.ApplicationURI = DefaultApplicationURI()
	}

	applicationURI, err := url.Parse(request.ApplicationURI)
	if err != nil {
		return nil, fmt.Errorf("invalid application URI %q: %w", request.ApplicationURI, err)
	}

	key, err := rsa.GenerateKey(rand.Reader, request.Bits)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}

	dnsNames := request.DNSNames
	if len(dnsNames) == 0 {
		if host, err := os.Hostname(); err == nil && host != "" {
			dnsNames = []string{host}
		}
	}

	addresses := make([]net.IP, 0, len(request.IPAddresses))
	for _, text := range request.IPAddresses {
		address := net.ParseIP(strings.TrimSpace(text))
		if address == nil {
			return nil, fmt.Errorf("invalid IP address %q", text)
		}
		addresses = append(addresses, address)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   request.CommonName,
			Organization: []string{"opcuacli"},
		},
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(request.Validity),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDataEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		IPAddresses:           addresses,
		URIs:                  []*url.URL{applicationURI},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	return &KeyPair{
		CertDER: der,
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM: pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		}),
	}, nil
}

// PrivateKey decodes the generated private key, for a caller that needs the key
// itself rather than a file path.
func (k *KeyPair) PrivateKey() (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(k.KeyPEM)
	if block == nil {
		return nil, fmt.Errorf("generated key is not PEM encoded")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// Write saves a key pair into dir as cert.pem and key.pem. The key is written
// 0600; refusing to overwrite is the default because a replaced certificate has
// to be re-approved on every server that trusted the old one.
func (k *KeyPair) Write(dir string, force bool) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	k.CertPath = filepath.Join(dir, "cert.pem")
	k.KeyPath = filepath.Join(dir, "key.pem")

	for _, file := range []struct {
		path    string
		content []byte
		mode    os.FileMode
	}{
		{k.CertPath, k.CertPEM, 0o644},
		{k.KeyPath, k.KeyPEM, 0o600},
	} {
		if !force {
			if _, err := os.Stat(file.path); err == nil {
				return fmt.Errorf("%s already exists; pass --force to replace it", file.path)
			}
		}
		if err := os.WriteFile(file.path, file.content, file.mode); err != nil {
			return fmt.Errorf("write %s: %w", file.path, err)
		}
	}

	return nil
}

// Ensure returns the paths of a usable client key pair in dir, generating one if
// it is not there yet. It reports whether it had to generate, so a caller can
// say so: the new certificate is untrusted until an operator approves it.
func Ensure(dir, applicationURI string) (certPath, keyPath string, generated bool, err error) {
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if certErr == nil && keyErr == nil {
		return certPath, keyPath, false, nil
	}

	pair, err := Generate(Request{ApplicationURI: applicationURI})
	if err != nil {
		return "", "", false, err
	}
	if err := pair.Write(dir, true); err != nil {
		return "", "", false, err
	}

	return pair.CertPath, pair.KeyPath, true, nil
}

// Load reads a certificate from a PEM or DER file and returns its DER bytes.
func Load(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeCertificate(raw)
}

// DecodeCertificate accepts either PEM or bare DER, since both spellings are
// handed out by servers and tools.
func DecodeCertificate(raw []byte) ([]byte, error) {
	if block, _ := pem.Decode(raw); block != nil {
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("expected a CERTIFICATE block, found %s", block.Type)
		}
		return block.Bytes, nil
	}
	if _, err := x509.ParseCertificate(raw); err != nil {
		return nil, fmt.Errorf("not a PEM or DER certificate: %w", err)
	}
	return raw, nil
}

// Describe renders a certificate as the declared output type. path is recorded
// when the certificate came from a file.
func Describe(der []byte, path string) (cligen.CertificateInfo, error) {
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return cligen.CertificateInfo{}, fmt.Errorf("parse certificate: %w", err)
	}

	sha1Sum := sha1.Sum(der)
	sha256Sum := sha256.Sum256(der)

	info := cligen.CertificateInfo{
		Subject:            certificate.Subject.String(),
		Issuer:             certificate.Issuer.String(),
		NotBefore:          certificate.NotBefore,
		NotAfter:           certificate.NotAfter,
		SerialNumber:       new(certificate.SerialNumber.String()),
		SignatureAlgorithm: new(certificate.SignatureAlgorithm.String()),
		PublicKeyAlgorithm: new(certificate.PublicKeyAlgorithm.String()),
		ThumbprintSha1:     new(hex.EncodeToString(sha1Sum[:])),
		ThumbprintSha256:   new(hex.EncodeToString(sha256Sum[:])),
		Expired:            new(time.Now().After(certificate.NotAfter) || time.Now().Before(certificate.NotBefore)),
		SelfSigned:         new(certificate.Subject.String() == certificate.Issuer.String()),
		DNSNames:           certificate.DNSNames,
		KeyUsage:           keyUsageNames(certificate.KeyUsage),
		ExtendedKeyUsage:   extendedKeyUsageNames(certificate),
	}

	if path != "" {
		info.Path = new(path)
	}
	if key, ok := certificate.PublicKey.(*rsa.PublicKey); ok {
		info.KeyBits = new(key.N.BitLen())
	}
	for _, uri := range certificate.URIs {
		info.ApplicationURI = new(uri.String())
		break
	}
	for _, address := range certificate.IPAddresses {
		info.IPAddresses = append(info.IPAddresses, address.String())
	}

	return info, nil
}

func keyUsageNames(usage x509.KeyUsage) []string {
	all := []struct {
		bit  x509.KeyUsage
		name string
	}{
		{x509.KeyUsageDigitalSignature, "DigitalSignature"},
		{x509.KeyUsageContentCommitment, "ContentCommitment"},
		{x509.KeyUsageKeyEncipherment, "KeyEncipherment"},
		{x509.KeyUsageDataEncipherment, "DataEncipherment"},
		{x509.KeyUsageKeyAgreement, "KeyAgreement"},
		{x509.KeyUsageCertSign, "CertSign"},
		{x509.KeyUsageCRLSign, "CRLSign"},
		{x509.KeyUsageEncipherOnly, "EncipherOnly"},
		{x509.KeyUsageDecipherOnly, "DecipherOnly"},
	}

	var names []string
	for _, candidate := range all {
		if usage&candidate.bit != 0 {
			names = append(names, candidate.name)
		}
	}
	return names
}

func extendedKeyUsageNames(certificate *x509.Certificate) []string {
	names := make([]string, 0, len(certificate.ExtKeyUsage))
	for _, usage := range certificate.ExtKeyUsage {
		switch usage {
		case x509.ExtKeyUsageClientAuth:
			names = append(names, "ClientAuth")
		case x509.ExtKeyUsageServerAuth:
			names = append(names, "ServerAuth")
		case x509.ExtKeyUsageAny:
			names = append(names, "Any")
		default:
			names = append(names, fmt.Sprintf("Unknown(%d)", usage))
		}
	}
	for _, unknown := range certificate.UnknownExtKeyUsage {
		names = append(names, unknown.String())
	}
	return names
}

// Validate reports why a certificate is unfit for use, so a connection can fail
// with the reason rather than a handshake error. An expired or not-yet-valid
// certificate is the usual cause of a rejected secure channel.
func Validate(der []byte) error {
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}

	now := time.Now()
	switch {
	case now.Before(certificate.NotBefore):
		return fmt.Errorf("certificate is not valid until %s", certificate.NotBefore.Format(time.RFC3339))
	case now.After(certificate.NotAfter):
		return fmt.Errorf("certificate expired on %s", certificate.NotAfter.Format(time.RFC3339))
	}
	return nil
}
