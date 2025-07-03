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

const (
	KeyBits  = 2048
	RsaParts = 2
)

// Generate RSA key pair
func GenerateRSAKeys() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, KeyBits)
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
	if len(parts) != RsaParts {
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

func RSAEncryptBytes(publicKey string, message []byte) ([]byte, error) {
	publicKeyPEM, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key: %v", err)
	}

	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the public key")
	}

	publicKeyParsed, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	// Generate a random AES key
	aesKey := make([]byte, 32) // AES-256
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %v", err)
	}

	// Encrypt the message using AES
	blockA, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	ciphertext := make([]byte, len(message))
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %v", err)
	}

	stream := cipher.NewCFBEncrypter(blockA, iv)
	stream.XORKeyStream(ciphertext, message)

	// Encrypt the AES key using RSA
	encryptedAESKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKeyParsed, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt AES key: %v", err)
	}

	// Combine the encrypted AES key and the ciphertext
	result := append(encryptedAESKey, iv...)
	result = append(result, ciphertext...)

	return result, nil
}