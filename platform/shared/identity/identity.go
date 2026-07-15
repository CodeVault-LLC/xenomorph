// Package identity derives stable agent identifiers from certificate material.
//
// The gateway owns the resulting identity assertion because it calls this
// package only after mutual TLS verification. An agent may derive the same
// value from its local certificate solely to verify command audience binding;
// that local value is not authoritative at the gateway.
package identity

import (
	"crypto/sha256"
	"crypto/x509"
	"fmt"
)

const (
	versionValueMask = byte(0x0f)
	versionFiveBits  = byte(0x50)
	variantValueMask = byte(0x3f)
	rfc4122Variant   = byte(0x80)
)

// AgentIDFromCertificate returns the stable agent ID derived from a certificate.
func AgentIDFromCertificate(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", fmt.Errorf("certificate is nil")
	}

	if len(cert.Raw) == 0 {
		return "", fmt.Errorf("certificate has empty raw bytes")
	}

	fingerprint := sha256.Sum256(cert.Raw)
	id := fingerprint[:16]
	id[6] = (id[6] & versionValueMask) | versionFiveBits
	id[8] = (id[8] & variantValueMask) | rfc4122Variant

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}
