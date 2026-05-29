// Package for all internal cryptographic operations
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

// Encrypt a string using a password with chacha20poly1305 and return a byte array of cipher text with required salt and nonce
func Encrypt(plainTextBytes []byte, decryptPassword []byte) (cipherTextSaltNonce []byte, err error) {
	// Generate a salt
	salt := make([]byte, 16) // 16 bytes salt
	if _, err = io.ReadFull(rand.Reader, salt); err != nil {
		return
	}

	// Derive the encryption key using Argon2
	key := deriveKey(decryptPassword, salt)

	// Create a new ChaCha20-Poly1305 instance
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return
	}

	// Generate a nonce (12 bytes for ChaCha20-Poly1305)
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return
	}

	// Encrypt the plaintext
	ciphertext := aead.Seal(plainTextBytes[:0], nonce, plainTextBytes, nil)

	// The final ciphertext will include the salt and nonce for later decryption
	cipherTextSaltNonce = append(salt, append(nonce, ciphertext...)...)

	// Encode byte array to base64
	encodedCipherText := base64.StdEncoding.EncodeToString(cipherTextSaltNonce)
	cipherTextSaltNonce = []byte(encodedCipherText)

	return
}

// Decrypt a byte array using a password with chacha20poly1305 and return a string of plain text
func Decrypt(cipherTextSaltNonce []byte, encryptPassword []byte) (plainText string, err error) {
	// Decode base64 to raw byte array
	cipherTextSaltNonce, err = base64.StdEncoding.DecodeString(string(cipherTextSaltNonce))
	if err != nil {
		err = fmt.Errorf("failed to decode cipher text from base64: %w", err)
		return
	}

	// Extract the salt (16 bytes) and nonce (12 bytes) from the ciphertext
	salt := cipherTextSaltNonce[:16]
	nonce := cipherTextSaltNonce[16:28]
	cipherTextBytes := cipherTextSaltNonce[28:]

	// Derive the decryption key using Argon2
	key := deriveKey(encryptPassword, salt)

	// Create a new ChaCha20-Poly1305 instance
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return
	}

	// Decrypt the ciphertext
	plainTextBytes, err := aead.Open(nil, nonce, cipherTextBytes, nil)
	if err != nil {
		return
	}

	plainText = string(plainTextBytes)
	return
}
