package encryption

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
)

// Generate RSA key pair
func GenerateRSAKeys() (*rsa.PrivateKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

// RSA Decrypt
func RsaDecrypt(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, ciphertext, nil)
}
