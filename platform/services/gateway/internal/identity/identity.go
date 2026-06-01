package identity

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// AuthenticatedAgent captures gateway-authored identity facts derived from a
// verified mTLS peer certificate. Every field in this struct is owned by the
// gateway trust boundary and must never be overwritten by client payload data.
type AuthenticatedAgent struct {
	// ID is a deterministic UUID generated from the certificate fingerprint.
	//
	// Properties:
	// - Stable for a given certificate.
	// - Distinct for different certificates.
	// - Cannot be spoofed without presenting the same certificate during mTLS.
	ID string

	// FingerprintSHA256 is the lowercase hex SHA-256 digest of cert.Raw.
	FingerprintSHA256 string

	// CertificateSerialNumber is copied from the verified client certificate.
	CertificateSerialNumber string

	// SubjectCommonName is preserved for observability and operator debugging.
	SubjectCommonName string
}

// FromPeerCertificate constructs gateway-trusted agent identity metadata from
// a verified client certificate.
func FromPeerCertificate(cert *x509.Certificate) (AuthenticatedAgent, error) {
	if cert == nil {
		return AuthenticatedAgent{}, fmt.Errorf("missing peer certificate")
	}
	if len(cert.Raw) == 0 {
		return AuthenticatedAgent{}, fmt.Errorf("peer certificate has empty raw bytes")
	}

	fingerprint := sha256.Sum256(cert.Raw)
	deterministicID := uuidFromFingerprint(fingerprint)

	return AuthenticatedAgent{
		ID:                      deterministicID.String(),
		FingerprintSHA256:       hex.EncodeToString(fingerprint[:]),
		CertificateSerialNumber: cert.SerialNumber.String(),
		SubjectCommonName:       strings.TrimSpace(cert.Subject.CommonName),
	}, nil
}

func uuidFromFingerprint(fingerprint [32]byte) uuid.UUID {
	var id uuid.UUID
	copy(id[:], fingerprint[:16])
	id[6] = (id[6] & 0x0f) | 0x50 // RFC4122 version 5 style marker.
	id[8] = (id[8] & 0x3f) | 0x80 // RFC4122 variant.
	return id
}
