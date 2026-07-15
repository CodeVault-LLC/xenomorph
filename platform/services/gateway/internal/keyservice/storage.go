package keyservice

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

const (
	privateKeyPermission   os.FileMode = 0o600
	publicKeyPermission    os.FileMode = 0o644
	keyDirectoryMode       os.FileMode = 0o700
	commandKeyLifetime                 = 90 * 24 * time.Hour
	maxCommandKeyFileBytes             = 64 << 10
)

// LoadOrCreateCommandSigner loads a dedicated RSA-PSS command key or creates
// one with crypto/rand when the configured path does not exist. It publishes
// only the corresponding PKIX public key and never reads the TLS private key.
func (service *Service) LoadOrCreateCommandSigner(ctx context.Context, privatePath, publicPath string, version uint64) (*CommandSigner, error) {
	if err := service.beforeOperation(ctx); err != nil {
		return nil, err
	}

	if err := validateCommandKeyPaths(privatePath, publicPath, version); err != nil {
		return nil, err
	}

	privateKey, createdAt, err := loadCommandPrivateKey(privatePath)
	if errors.Is(err, os.ErrNotExist) {
		return service.createAndStoreCommandSigner(ctx, privatePath, publicPath, version)
	}

	if err != nil {
		return nil, err
	}

	signer, err := commandSignerFromPrivateKey(privateKey, version, createdAt)
	if err != nil {
		return nil, err
	}

	if err := signer.Ready(); err != nil {
		return nil, err
	}

	if err := writePublicKey(publicPath, &privateKey.PublicKey); err != nil {
		return nil, err
	}

	service.setCommandSigner(signer)

	return signer, nil
}

func validateCommandKeyPaths(privatePath, publicPath string, version uint64) error {
	if privatePath == "" || publicPath == "" {
		return fmt.Errorf("command private and public key paths are required")
	}

	if version == 0 {
		return fmt.Errorf("command key version is required")
	}

	if sameCleanPath(privatePath, publicPath) {
		return fmt.Errorf("command private and public key paths must differ")
	}

	return nil
}

func (service *Service) createAndStoreCommandSigner(ctx context.Context, privatePath, publicPath string, version uint64) (*CommandSigner, error) {
	signer, err := service.GenerateCommandSigner(ctx, version)
	if err != nil {
		return nil, err
	}

	privateKey, ok := signer.signer.(*rsa.PrivateKey)
	if !ok {
		signer.Destroy()
		return nil, fmt.Errorf("software command signer does not contain an RSA private key")
	}

	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		signer.Destroy()
		return nil, fmt.Errorf("encode command signing key: %w", err)
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
	if err := writeSecretFile(privatePath, privatePEM); err != nil {
		clear(privatePEM)
		signer.Destroy()

		return nil, err
	}

	clear(privatePEM)

	if err := writePublicKey(publicPath, &privateKey.PublicKey); err != nil {
		signer.Destroy()
		return nil, err
	}

	if err := signer.Activate(); err != nil {
		signer.Destroy()
		return nil, err
	}

	service.setCommandSigner(signer)

	return signer, nil
}

func loadCommandPrivateKey(path string) (*rsa.PrivateKey, time.Time, error) {
	info, err := os.Lstat(filepath.Clean(path))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("inspect command signing key: %w", err)
	}

	encoded, openedInfo, err := readPrivateKeyFile(filepath.Clean(path), info)
	if err != nil {
		return nil, time.Time{}, err
	}

	defer clear(encoded)

	block, remainder := pem.Decode(encoded)
	if block == nil || len(remainder) != 0 {
		return nil, time.Time{}, fmt.Errorf("decode command signing key: invalid PEM data")
	}

	privateKey, err := parseCommandPrivateKey(block.Bytes)
	if err != nil {
		return nil, time.Time{}, err
	}

	return privateKey, openedInfo.ModTime().UTC(), nil
}

func readPrivateKeyFile(path string, initialInfo os.FileInfo) ([]byte, os.FileInfo, error) {
	if err := validatePrivateKeyFile(initialInfo); err != nil {
		return nil, nil, err
	}

	file, err := os.Open(path) // #nosec G304 -- operator-configured secret-file path.
	if err != nil {
		return nil, nil, fmt.Errorf("open command signing key: %w", err)
	}

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("inspect opened command signing key: %w", err), file.Close())
	}

	if !os.SameFile(initialInfo, openedInfo) {
		return nil, nil, errors.Join(fmt.Errorf("command signing key changed while opening"), file.Close())
	}

	encoded, err := io.ReadAll(io.LimitReader(file, maxCommandKeyFileBytes+1))
	closeErr := file.Close()

	if err != nil {
		clear(encoded)
		return nil, nil, errors.Join(fmt.Errorf("read command signing key: %w", err), closeErr)
	}

	if closeErr != nil {
		clear(encoded)
		return nil, nil, fmt.Errorf("close command signing key: %w", closeErr)
	}

	if len(encoded) > maxCommandKeyFileBytes {
		clear(encoded)
		return nil, nil, fmt.Errorf("command signing key file exceeds size limit")
	}

	return encoded, openedInfo, nil
}

func validatePrivateKeyFile(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("command signing key must be a regular file")
	}

	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("command signing key must be owner-only")
	}

	return nil
}

func parseCommandPrivateKey(encoded []byte) (*rsa.PrivateKey, error) {
	parsed, err := x509.ParsePKCS8PrivateKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("parse command signing key: %w", err)
	}

	privateKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("command signing key must be RSA")
	}

	if privateKey.N.BitLen() < commandKeyBits {
		return nil, fmt.Errorf("command signing key must contain at least %d bits", commandKeyBits)
	}

	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("validate command signing key: %w", err)
	}

	return privateKey, nil
}

func commandSignerFromPrivateKey(privateKey *rsa.PrivateKey, version uint64, createdAt time.Time) (*CommandSigner, error) {
	keyID, err := commandauth.KeyID(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}

	return &CommandSigner{metadata: Metadata{
		ID: keyID, Algorithm: "RSA-PSS-SHA256", Purpose: "command-signing",
		Version: version, State: StateActive, CreatedAt: createdAt,
		NotAfter: createdAt.Add(commandKeyLifetime),
	}, signer: privateKey}, nil
}

func writePublicKey(path string, publicKey *rsa.PublicKey) error {
	encoded, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("encode command verification key: %w", err)
	}

	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: encoded})

	return writeAtomicFile(path, publicPEM, publicKeyPermission)
}

func writeSecretFile(path string, data []byte) error {
	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(cleanPath), keyDirectoryMode); err != nil {
		return fmt.Errorf("create command key directory: %w", err)
	}

	file, err := os.OpenFile(cleanPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateKeyPermission) // #nosec G304 -- operator-configured secret-file path.
	if err != nil {
		return fmt.Errorf("create command signing key: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write command signing key: %w", err), discardFile(file, cleanPath))
	}

	if err := file.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync command signing key: %w", err), discardFile(file, cleanPath))
	}

	if err := file.Close(); err != nil {
		return errors.Join(fmt.Errorf("close command signing key: %w", err), removeFile(cleanPath))
	}

	return nil
}

func writeAtomicFile(path string, data []byte, permission os.FileMode) (returnErr error) {
	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(cleanPath), keyDirectoryMode); err != nil {
		return fmt.Errorf("create public key directory: %w", err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(cleanPath), ".command-public-*")
	if err != nil {
		return fmt.Errorf("create temporary public key: %w", err)
	}

	temporaryPath := temporary.Name()
	removeTemporary := true

	defer func() {
		if removeTemporary {
			returnErr = errors.Join(returnErr, removeFile(temporaryPath))
		}
	}()

	if err := temporary.Chmod(permission); err != nil {
		return errors.Join(fmt.Errorf("set public key permissions: %w", err), temporary.Close())
	}

	if _, err := temporary.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write public key: %w", err), temporary.Close())
	}

	if err := temporary.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync public key: %w", err), temporary.Close())
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close public key: %w", err)
	}

	if err := os.Rename(temporaryPath, cleanPath); err != nil {
		return fmt.Errorf("publish command verification key: %w", err)
	}

	removeTemporary = false

	return nil
}

func discardFile(file *os.File, path string) error {
	return errors.Join(file.Close(), removeFile(path))
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func sameCleanPath(first, second string) bool {
	firstAbsolute, firstErr := filepath.Abs(filepath.Clean(first))
	secondAbsolute, secondErr := filepath.Abs(filepath.Clean(second))

	return firstErr == nil && secondErr == nil && firstAbsolute == secondAbsolute
}
