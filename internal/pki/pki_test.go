package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateProducesAUsableClientCertificate(t *testing.T) {
	pair, err := Generate(Request{
		CommonName:     "opcuacli-test",
		ApplicationURI: "urn:test:opcuacli",
		DNSNames:       []string{"plant.example"},
		IPAddresses:    []string{"10.0.0.1"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	certificate, err := x509.ParseCertificate(pair.CertDER)
	if err != nil {
		t.Fatalf("the generated certificate does not parse: %v", err)
	}

	// A server matches the application URI in a subject alternative name against
	// the URI the client announces; without it the channel is refused.
	if len(certificate.URIs) != 1 || certificate.URIs[0].String() != "urn:test:opcuacli" {
		t.Errorf("URIs = %v, want the application URI", certificate.URIs)
	}
	if certificate.Subject.CommonName != "opcuacli-test" {
		t.Errorf("common name = %q", certificate.Subject.CommonName)
	}
	if len(certificate.DNSNames) != 1 || certificate.DNSNames[0] != "plant.example" {
		t.Errorf("DNS names = %v", certificate.DNSNames)
	}
	if len(certificate.IPAddresses) != 1 {
		t.Errorf("IP addresses = %v", certificate.IPAddresses)
	}

	// A secure channel needs both key agreement and signing.
	for _, usage := range []x509.KeyUsage{x509.KeyUsageDigitalSignature, x509.KeyUsageKeyEncipherment} {
		if certificate.KeyUsage&usage == 0 {
			t.Errorf("key usage %v is missing", usage)
		}
	}

	if _, err := pair.PrivateKey(); err != nil {
		t.Errorf("PrivateKey: %v", err)
	}
}

func TestGenerateRejectsAShortKey(t *testing.T) {
	if _, err := Generate(Request{Bits: 1024}); err == nil {
		t.Error("Generate accepted a 1024-bit key, which every policy refuses")
	}
}

func TestGenerateRejectsABadApplicationURI(t *testing.T) {
	if _, err := Generate(Request{ApplicationURI: "://nonsense"}); err == nil {
		t.Error("Generate accepted an unparseable application URI")
	}
}

func TestWriteRefusesToOverwriteWithoutForce(t *testing.T) {
	directory := t.TempDir()

	pair, err := Generate(Request{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := pair.Write(directory, false); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// A replaced certificate has to be re-approved on every server that trusted
	// the old one, so overwriting is deliberate.
	if err := pair.Write(directory, false); err == nil {
		t.Error("Write replaced an existing certificate without --force")
	}
	if err := pair.Write(directory, true); err != nil {
		t.Errorf("Write with force: %v", err)
	}

	_, err := os.Stat(filepath.Join(directory, "key.pem"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
}

func TestEnsureGeneratesOnceAndThenReuses(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "certs")

	certPath, keyPath, generated, err := Ensure(directory, "urn:test:opcuacli")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !generated {
		t.Error("the first call did not report generating a key pair")
	}

	first, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read the generated certificate: %v", err)
	}

	againCert, againKey, generated, err := Ensure(directory, "urn:test:opcuacli")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if generated {
		t.Error("the second call generated a new key pair; the certificate must be stable across runs")
	}
	if againCert != certPath || againKey != keyPath {
		t.Error("Ensure returned different paths the second time")
	}

	second, err := os.ReadFile(againCert)
	if err != nil {
		t.Fatalf("read the certificate again: %v", err)
	}
	if string(first) != string(second) {
		t.Error("the certificate changed between calls")
	}
}

func TestLoadAcceptsPEMAndDER(t *testing.T) {
	directory := t.TempDir()

	pair, err := Generate(Request{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	pemPath := filepath.Join(directory, "cert.pem")
	if err := os.WriteFile(pemPath, pair.CertPEM, 0o644); err != nil {
		t.Fatalf("write PEM: %v", err)
	}
	derPath := filepath.Join(directory, "cert.der")
	if err := os.WriteFile(derPath, pair.CertDER, 0o644); err != nil {
		t.Fatalf("write DER: %v", err)
	}

	for _, path := range []string{pemPath, derPath} {
		der, err := Load(path)
		if err != nil {
			t.Fatalf("Load(%s): %v", filepath.Base(path), err)
		}
		if len(der) != len(pair.CertDER) {
			t.Errorf("Load(%s) returned %d bytes, want %d", filepath.Base(path), len(der), len(pair.CertDER))
		}
	}

	junk := filepath.Join(directory, "junk.pem")
	if err := os.WriteFile(junk, []byte("not a certificate"), 0o644); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	if _, err := Load(junk); err == nil {
		t.Error("Load accepted a file that is not a certificate")
	}
}

func TestDescribe(t *testing.T) {
	pair, err := Generate(Request{CommonName: "described", ApplicationURI: "urn:test:described"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	info, err := Describe(pair.CertDER, "/tmp/cert.pem")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	if !strings.Contains(info.Subject, "described") {
		t.Errorf("subject = %q", info.Subject)
	}
	if info.ApplicationURI == nil || *info.ApplicationURI != "urn:test:described" {
		t.Errorf("application URI = %v", info.ApplicationURI)
	}
	if info.SelfSigned == nil || !*info.SelfSigned {
		t.Error("a self-signed certificate was not reported as one")
	}
	if info.Expired == nil || *info.Expired {
		t.Error("a fresh certificate was reported as expired")
	}
	if info.KeyBits == nil || *info.KeyBits != 2048 {
		t.Errorf("key bits = %v, want 2048", info.KeyBits)
	}
	if info.ThumbprintSha256 == nil || len(*info.ThumbprintSha256) != 64 {
		t.Errorf("SHA-256 thumbprint = %v", info.ThumbprintSha256)
	}
	if info.Path == nil || *info.Path != "/tmp/cert.pem" {
		t.Errorf("path = %v", info.Path)
	}
}

// An expired certificate is the usual reason a secure channel is refused, so it
// is worth failing on before the handshake rather than after.
func TestValidateRejectsAnExpiredCertificate(t *testing.T) {
	fresh, err := Generate(Request{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := Validate(fresh.CertDER); err != nil {
		t.Errorf("Validate rejected a fresh certificate: %v", err)
	}

	if err := Validate(expiredCertificate(t)); err == nil {
		t.Error("Validate accepted a certificate whose window has passed")
	}

	if err := Validate([]byte("not a certificate")); err == nil {
		t.Error("Validate accepted something that is not a certificate")
	}
}

// expiredCertificate builds a certificate whose window closed yesterday.
// Generate cannot produce one - it always backdates NotBefore by an hour and
// dates NotAfter forward - so the template is written out here.
func expiredCertificate(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return der
}

func TestDefaultApplicationURIIsAURN(t *testing.T) {
	if got := DefaultApplicationURI(); !strings.HasPrefix(got, "urn:") || !strings.HasSuffix(got, ":opcuacli") {
		t.Errorf("DefaultApplicationURI = %q, want urn:<host>:opcuacli", got)
	}
}
