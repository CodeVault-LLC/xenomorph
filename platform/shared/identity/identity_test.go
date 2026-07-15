package identity

import (
	"crypto/x509"
	"testing"
)

func TestAgentIDFromCertificateStable(t *testing.T) {
	t.Parallel()

	cert := &x509.Certificate{Raw: []byte("certificate")}

	first, err := AgentIDFromCertificate(cert)
	if err != nil {
		t.Fatalf("AgentIDFromCertificate() error = %v", err)
	}

	second, err := AgentIDFromCertificate(cert)
	if err != nil {
		t.Fatalf("AgentIDFromCertificate() second error = %v", err)
	}

	if first != second {
		t.Fatalf("AgentIDFromCertificate() = %q, want stable value %q", second, first)
	}
}

func TestAgentIDFromCertificateRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cert *x509.Certificate
	}{
		{name: "nil certificate"},
		{name: "empty certificate", cert: &x509.Certificate{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := AgentIDFromCertificate(test.cert); err == nil {
				t.Fatal("AgentIDFromCertificate() error = nil, want error")
			}
		})
	}
}
