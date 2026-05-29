package helpers

import (
	"crypto/sha256"
	"encoding/hex"
)

func Hash(content []byte) (hexHash string) {
	hasher := sha256.New()
	hasher.Write(content)
	hashBytes := hasher.Sum(nil)
	hexHash = hex.EncodeToString(hashBytes)
	return
}
