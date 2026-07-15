package agentquic

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/shared/atomicfile"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const (
	secretKeyBytes      = 32
	secretDirectoryMode = 0o700
	secretFileMode      = 0o600
)

func buildServerTLSConfig(config Config, _ *handshakeAdmission, metrics *Metrics) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(config.ServerCertificateFile, config.ServerPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load QUIC server certificate: %w", err)
	}

	caData, err := os.ReadFile(filepath.Clean(config.ClientCAFile))
	if err != nil {
		return nil, fmt.Errorf("read QUIC client CA: %w", err)
	}

	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("parse QUIC client CA: invalid PEM data")
	}

	base := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    clientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{wire.ALPN},
	}
	base.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		connectionConfig := base.Clone()
		connectionConfig.GetConfigForClient = nil
		ticket := ticketFromContext(hello.Context())
		connectionConfig.VerifyConnection = func(state tls.ConnectionState) error {
			defer ticket.release()

			if err := validatePeerCertificateState(state, config); err != nil {
				metrics.certificateFailed.Add(1)
				return err
			}

			return nil
		}

		return connectionConfig, nil
	}

	return base, nil
}

func validatePeerCertificateState(state tls.ConnectionState, config Config) error {
	if state.Version != tls.VersionTLS13 || state.NegotiatedProtocol != wire.ALPN {
		return fmt.Errorf("validate QUIC peer: TLS version or ALPN mismatch")
	}

	if len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return fmt.Errorf("validate QUIC peer: verified client certificate required")
	}

	chainBytes := 0
	for _, certificate := range state.PeerCertificates {
		chainBytes += len(certificate.Raw)
		if chainBytes > config.MaximumClientChainBytes {
			return fmt.Errorf("validate QUIC peer: certificate chain exceeds byte limit")
		}
	}

	for _, chain := range state.VerifiedChains {
		if len(chain) > config.MaximumClientChainDepth {
			return fmt.Errorf("validate QUIC peer: certificate chain exceeds depth limit")
		}
	}

	return nil
}

func loadTransportKeys(config Config) (*quic.StatelessResetKey, *quic.TokenGeneratorKey, error) {
	resetBytes, err := loadOrCreateSecretKey(config.StatelessResetKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load stateless reset key: %w", err)
	}

	tokenBytes, err := loadOrCreateSecretKey(config.TokenGeneratorKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load token generator key: %w", err)
	}

	resetKey := quic.StatelessResetKey(resetBytes)
	tokenKey := quic.TokenGeneratorKey(tokenBytes)

	return &resetKey, &tokenKey, nil
}

func loadOrCreateSecretKey(path string) ([secretKeyBytes]byte, error) {
	existing, err := loadSecretKey(path)
	if err == nil {
		return existing, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return [secretKeyBytes]byte{}, err
	}

	var generated [secretKeyBytes]byte
	if _, err := rand.Read(generated[:]); err != nil {
		return [secretKeyBytes]byte{}, fmt.Errorf("generate transport secret key: %w", err)
	}

	encoded := make([]byte, hex.EncodedLen(len(generated))+1)
	hex.Encode(encoded, generated[:])
	encoded[len(encoded)-1] = '\n'

	defer func() {
		clear(generated[:])
		clear(encoded)
	}()

	if err := atomicfile.Create(path, encoded, secretDirectoryMode, secretFileMode); err != nil && !errors.Is(err, fs.ErrExist) {
		return [secretKeyBytes]byte{}, fmt.Errorf("persist transport secret key: %w", err)
	}

	return loadSecretKey(path)
}

func loadSecretKey(path string) ([secretKeyBytes]byte, error) {
	var result [secretKeyBytes]byte
	if strings.TrimSpace(path) == "" {
		return result, fmt.Errorf("secret key path is required")
	}

	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return result, err
	}

	if info.Mode().Perm()&0o077 != 0 {
		return result, fmt.Errorf("secret key file permissions must exclude group and others")
	}

	encoded, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return result, err
	}

	encodedText := strings.TrimSpace(string(encoded))
	if len(encodedText) != hex.EncodedLen(secretKeyBytes) {
		return result, fmt.Errorf("secret key must contain exactly %d hexadecimal characters", hex.EncodedLen(secretKeyBytes))
	}

	decoded := make([]byte, secretKeyBytes)
	if _, err := hex.Decode(decoded, []byte(encodedText)); err != nil {
		return result, fmt.Errorf("decode hexadecimal key: %w", err)
	}

	copy(result[:], decoded)

	for index := range decoded {
		decoded[index] = 0
	}

	if result == [secretKeyBytes]byte{} {
		return [secretKeyBytes]byte{}, fmt.Errorf("secret key must be nonzero")
	}

	return result, nil
}
