package encryption

import (
	"crypto/aes"
	"crypto/cipher"
)

// AES Decrypt
func AesDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	nonceSize := 12 // AES-GCM uses a 12-byte nonce
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
