package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func TestFromPeerCertificateDeterministic(t *testing.T) {
	cert := mustBuildCertificate(t, "edge-01", 100)

	first, err := FromPeerCertificate(cert)
	if err != nil {
		t.Fatalf("first derivation failed: %v", err)
	}
	second, err := FromPeerCertificate(cert)
	if err != nil {
		t.Fatalf("second derivation failed: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected deterministic ID, got %q and %q", first.ID, second.ID)
	}
	if first.FingerprintSHA256 != second.FingerprintSHA256 {
		t.Fatalf("expected deterministic fingerprint, got %q and %q", first.FingerprintSHA256, second.FingerprintSHA256)
	}
	if first.SubjectCommonName != "edge-01" {
		t.Fatalf("unexpected common name: %q", first.SubjectCommonName)
	}
}

func TestFromPeerCertificateUniquePerCertificate(t *testing.T) {
	certA := mustBuildCertificate(t, "edge-01", 100)
	certB := mustBuildCertificate(t, "edge-02", 101)

	identityA, err := FromPeerCertificate(certA)
	if err != nil {
		t.Fatalf("identity A derivation failed: %v", err)
	}
	identityB, err := FromPeerCertificate(certB)
	if err != nil {
		t.Fatalf("identity B derivation failed: %v", err)
	}

	if identityA.ID == identityB.ID {
		t.Fatalf("expected different IDs for different certs: %q", identityA.ID)
	}
	if identityA.FingerprintSHA256 == identityB.FingerprintSHA256 {
		t.Fatalf("expected different fingerprints for different certs")
	}
}

func TestFromPeerCertificateRejectsNil(t *testing.T) {
	_, err := FromPeerCertificate(nil)
	if err == nil {
		t.Fatal("expected error for nil certificate")
	}
}

func mustBuildCertificate(t *testing.T, commonName string, serial int64) *x509.Certificate {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	return cert
}
