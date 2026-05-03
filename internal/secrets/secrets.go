package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func GenerateClientToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return "nk-" + hex.EncodeToString(b)
}

func GenerateAPIKey() (string, string) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	token := "nk-" + hex.EncodeToString(b)
	id := hex.EncodeToString(b[:4])
	return id, token
}
