// Package identity derives gateway-trusted agent identity from verified mTLS
// peer certificates. This package owns the identity trust boundary: the
// AuthenticatedAgent struct is the only representation of agent identity that
// downstream code should trust.
//
// All identity fields are server-authored or gateway-validated. Client-supplied
// identity fields (e.g. in JSON request bodies) must never be treated as
// authoritative.
package identity

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"

	sharedidentity "github.com/codevault-llc/xenomorph/platform/shared/identity"
)

// AuthenticatedAgent captures gateway-authored identity facts derived from a
// verified mTLS peer certificate. Every field in this struct is owned by the
// gateway trust boundary and must never be overwritten by client payload data.
type AuthenticatedAgent struct {
	// ID is a deterministic UUID generated from the certificate fingerprint.
	//
	// Properties:
	//   - Stable for a given certificate across gateway restarts.
	//   - Distinct for different certificates.
	//   - Cannot be spoofed without presenting the same certificate during
	//     mTLS handshake.
	ID string

	// FingerprintSHA256 is the lowercase hex SHA-256 digest of cert.Raw.
	FingerprintSHA256 string

	// CertificateSerialNumber is copied from the verified client certificate.
	CertificateSerialNumber string

	// SubjectCommonName is preserved for observability and operator debugging.
	// This field must not be used for authorization decisions; use ID instead.
	SubjectCommonName string
}

// FromPeerCertificate constructs gateway-trusted agent identity metadata from
// a verified client certificate. The agent ID is deterministically derived
// from the certificate's SHA-256 fingerprint, ensuring the same certificate
// always produces the same agent ID.
//
// The input cert must be non-nil and contain non-empty Raw bytes. Returns an
// error when either precondition is violated.
func FromPeerCertificate(cert *x509.Certificate) (AuthenticatedAgent, error) {
	if cert == nil {
		return AuthenticatedAgent{}, fmt.Errorf("peer certificate is nil")
	}

	if len(cert.Raw) == 0 {
		return AuthenticatedAgent{}, fmt.Errorf("peer certificate has empty raw bytes")
	}

	fingerprint := sha256.Sum256(cert.Raw)

	deterministicID, err := sharedidentity.AgentIDFromCertificate(cert)
	if err != nil {
		return AuthenticatedAgent{}, fmt.Errorf("derive agent ID: %w", err)
	}

	return AuthenticatedAgent{
		ID:                      deterministicID,
		FingerprintSHA256:       hex.EncodeToString(fingerprint[:]),
		CertificateSerialNumber: cert.SerialNumber.String(),
		SubjectCommonName:       strings.TrimSpace(cert.Subject.CommonName),
	}, nil
}
