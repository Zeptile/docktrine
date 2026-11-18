// Package secrets encrypts secret values at rest with AES-256-GCM.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Box struct {
	aead cipher.AEAD
}

// LoadBox reads the master key from DOCKTRINE_SECRET_KEY (64 hex chars),
// or from <dataPath>/secret.key, generating that file on first use.
func LoadBox(dataPath string) (*Box, error) {
	keyHex := os.Getenv("DOCKTRINE_SECRET_KEY")
	if keyHex == "" {
		keyPath := filepath.Join(dataPath, "secret.key")
		data, err := os.ReadFile(keyPath)
		if os.IsNotExist(err) {
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return nil, err
			}
			keyHex = hex.EncodeToString(key)
			if err := os.WriteFile(keyPath, []byte(keyHex+"\n"), 0600); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			keyHex = strings.TrimSpace(string(data))
		}
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, hex encoded")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (b *Box) Decrypt(ciphertext []byte) (string, error) {
	size := b.aead.NonceSize()
	if len(ciphertext) < size {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := b.aead.Open(nil, ciphertext[:size], ciphertext[size:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypting secret: %w", err)
	}
	return string(plaintext), nil
}
