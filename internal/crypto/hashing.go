package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// SHA256 Content Hashing
// Takes a string input, and returns a SHA256 hexadecimal hash string
func SHA256Sum(input []byte) (hash string) {
	// Create new hashing function
	hasher := sha256.New()

	// Write input bytes into hasher
	hasher.Write(input)

	// Retrieve the raw hash
	rawHash := hasher.Sum(nil)

	// Format raw hash into hex
	hash = hex.EncodeToString(rawHash)

	return
}

// SHA256 Stream Hashing
// Takes filepath, reads in globally defined amount in buffer, hashes
// Returns hexadecimal hash string
func SHA256SumStream(filePath string) (hash string, err error) {
	const hashingBufferSize int = 64 * 1024 // 64KB Buffer for stream hashing

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer func() {
		lerr := file.Close()
		if err == nil && lerr != nil {
			return
		}
	}()

	// Create a new SHA-256 hash object
	hashObject := sha256.New()

	// Read the file in chunks and update the hash
	buffer := make([]byte, hashingBufferSize)
	for {
		var bytesRead int
		bytesRead, err = file.Read(buffer)
		if err != nil && err != io.EOF {
			return
		}
		if bytesRead == 0 {
			err = nil // Ensure previous EOF error doesn't get returned
			break     // End of file
		}

		// Update the hash with the read data
		_, err = hashObject.Write(buffer[:bytesRead])
		if err != nil {
			return
		}
	}

	// Return the final hash in hexadecimal format
	hash = fmt.Sprintf("%x", hashObject.Sum(nil))

	// Guard against empty returns
	if hash == "" {
		err = fmt.Errorf("unknown error: empty hash")
		return
	}

	return
}
