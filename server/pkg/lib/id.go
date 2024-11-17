package lib

import (
	"crypto/rand"
	"encoding/hex"
	"log"
)

// GenerateID generates a random ID for a connection.
// Example: "f47ac10b-58cc-4372-a567-0e02b2c3d479"
func GenerateID() string {
	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	if err != nil {
		log.Fatal(err)
	}

	return hex.EncodeToString(bytes)
}
