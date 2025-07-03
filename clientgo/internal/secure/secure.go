package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

type Sec struct {
	aesgcm cipher.AEAD
	active bool
}

func New() *Sec {
	return &Sec{active: false}
}

// InitFromRawKey is used when server provides a symmetric key (e.g., via handshake)
func (s *Sec) InitFromRawKey(raw []byte) error {
	hash := sha256.Sum256(raw) // hash to ensure 32 bytes
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	s.aesgcm = aesgcm
	s.active = true
	return nil
}

func (s *Sec) IsActive() bool {
	return s.active
}

func (s *Sec) Encrypt(data []byte) ([]byte, error) {
	if !s.active {
		return data, nil // pass-through if not active
	}

	nonce := make([]byte, s.aesgcm.NonceSize())
	_, _ = rand.Read(nonce)

	ciphertext := s.aesgcm.Seal(nil, nonce, data, nil)
	return append(nonce, ciphertext...), nil
}

func (s *Sec) Decrypt(data []byte) ([]byte, error) {
	if !s.active {
		return data, nil // pass-through if not active
	}

	nonceSize := s.aesgcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	return s.aesgcm.Open(nil, nonce, ciphertext, nil)
}
