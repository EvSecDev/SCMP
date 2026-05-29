package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Derive a secure key from a password string using argon2
func deriveKey(password []byte, salt []byte) (derivedKey []byte) {
	// Argon2 parameters
	const time = 1
	const memory = 64 * 1024
	const threads = 4
	const keyLength = 32

	// Derive the key from the password
	derivedKey = argon2.IDKey(password, salt, time, memory, threads, keyLength)
	return
}

func HashUserPassword(password string) (hash string, err error) {
	// Ensure password meets complexity requirements
	if !IsValidPassword(password) {
		err = fmt.Errorf("password does not meet complexity requirements (must have: letter, digit, uppercase, special character)")
		return
	}

	// Parameters
	var (
		memory      uint32 = 64 * 1024 // 64 MB
		iterations  uint32 = 3
		parallelism uint8  = 2
		saltLength  uint32 = 16
		keyLength   uint32 = 32
	)

	// Generate a random salt
	salt := make([]byte, saltLength)
	if _, err = rand.Read(salt); err != nil {
		err = fmt.Errorf("failed to generate salt: %w", err)
		return
	}

	// Derive the key using Argon2id
	hashRaw := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	// Encode the hash and salt for storage
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hashRaw)

	// Return the full encoded hash in a recognizable format
	hash = fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, iterations, parallelism, b64Salt, b64Hash)

	return
}

func AuthorizeUserPassword(encodedHash string, password string) (isAuthorized bool, err error) {
	// Example format:
	// $argon2id$v=19$m=65536,t=3,p=2$<base64-salt>$<base64-hash>
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		err = fmt.Errorf("invalid hash format")
		return
	}

	var (
		// Algorithm is parts[1] (e.g., argon2id)
		// Version    is parts[2] (e.g., v=19)
		paramsPart = parts[3]
		saltBase64 = parts[4]
		hashBase64 = parts[5]
	)

	// Parse the parameters
	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err = fmt.Sscanf(paramsPart, "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		err = fmt.Errorf("invalid hash parameters: %w", err)
		return
	}

	// Decode salt and hash
	salt, err := base64.RawStdEncoding.DecodeString(saltBase64)
	if err != nil {
		err = fmt.Errorf("invalid base64 salt: %w", err)
		return
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(hashBase64)
	if err != nil {
		err = fmt.Errorf("invalid base64 hash: %w", err)
		return
	}

	// Derive the key from the input password using the same parameters
	keyLen := uint32(len(expectedHash))
	computedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLen)

	// Constant time compare
	if subtleCompare(computedHash, expectedHash) {
		isAuthorized = true
		return
	}

	return
}

// Compares two byte slices in constant time
func subtleCompare(a, b []byte) (identical bool) {
	if len(a) != len(b) {
		return false
	}

	result := subtle.ConstantTimeCompare(a, b)
	switch result {
	case 0:
		identical = false
	case 1:
		identical = true
	}
	return
}
