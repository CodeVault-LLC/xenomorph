package encryption

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
)

// Generate RSA key pair
func GenerateRSAKeys() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate RSA key: %v", err)
	}

	// Convert private key to PEM format
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes})

	// Export public key in PKCS#1 format
	publicKeyBytes := x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: publicKeyBytes})

	// Encode both keys as base64 strings (for safe transport)
	privateKeyStr := base64.StdEncoding.EncodeToString(privateKeyPEM)
	publicKeyStr := base64.StdEncoding.EncodeToString(publicKeyPEM)

	return publicKeyStr, privateKeyStr, nil
}

// RSA Decrypt
func RSADecrypt(privateKeyStr string, cipherText string) (string, error) {
	// Decode base64 strings
	privateKeyPEM, err := base64.StdEncoding.DecodeString(privateKeyStr)
	if err != nil {
		return "", fmt.Errorf("failed to decode private key: %v", err)
	}

	cipherTextBytes, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %v", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing the private key")
	}

	// Parse the key
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %v", err)
	}

	// Decrypt the data
	plainTextBytes, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, cipherTextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt data: %v", err)
	}

	return string(plainTextBytes), nil
}

func RSADecryptBytes(privateKey string, cipherText []byte) ([]byte, error) {
	privateKeyPEM, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %v", err)
	}

	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the private key")
	}

	privateKeyParsed, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	parts := bytes.Split(cipherText, []byte("||"))
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid data format")
	}
	encryptedAESKey := parts[0]
	encryptedMessage := parts[1]

	// Decrypt the AES key
	aesKey, err := rsa.DecryptOAEP(
		sha256.New(),
		rand.Reader,
		privateKeyParsed,
		encryptedAESKey,
		nil,
	)

	if err != nil {
		log.Printf("Error decrypting AES key: %v", err)
		return nil, err
	}

	// Decrypt the message using AES
	iv := encryptedMessage[:16]
	ciphertext := encryptedMessage[16:]
	blockA, err := aes.NewCipher(aesKey)
	if err != nil {
		log.Printf("Error creating AES cipher: %v", err)
		return nil, err
	}

	stream := cipher.NewCFBDecrypter(blockA, iv)
	plaintext := make([]byte, len(ciphertext))
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}
