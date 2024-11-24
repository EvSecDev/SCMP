// siggen
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/ssh/terminal"
)

func logError(message string, err error) {
	fmt.Printf("Error: %s: %v\n", message, err)
	os.Exit(1)
}

const (
	saltLength = 16
	keyLength  = 32 // Length of the key for ChaCha20-Poly1305
	iterations = 100000
)

func main() {
	var sourceFilePath string
	var privateKeyFilePath string
	var publicKeyFilePath string
	var signFlagExists bool
	var verifyFlagExists bool
	var genKeysFlagExists bool
	flag.BoolVar(&signFlagExists, "sign", false, "Generate Signature for file and write to sig")
	flag.BoolVar(&verifyFlagExists, "verify", false, "Verify file using pub and sig")
	flag.BoolVar(&genKeysFlagExists, "genkeys", false, "Generate new private and public ed25519 keys - use -priv and -pub for key output files")
	flag.StringVar(&sourceFilePath, "in", "", "Input file to generate sig or verify with sig")
	flag.StringVar(&privateKeyFilePath, "priv", "", "File path for the private key")
	flag.StringVar(&publicKeyFilePath, "pub", "", "File path for the public key")
	flag.Parse()

	if signFlagExists {
		signFile(sourceFilePath, privateKeyFilePath)
	}
	if verifyFlagExists {
		verifyFile(sourceFilePath, publicKeyFilePath)
	}
	if genKeysFlagExists {
		genKeys(privateKeyFilePath, publicKeyFilePath)
	}
}

func genKeys(privateKeyFilePath string, publicKeyFilePath string) {
	// Generate the keys
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		logError("Failed to generate keys", err)
	}

	// Convert to base64 and then convert to byte array
	pubKeyb := []byte(base64.StdEncoding.EncodeToString(pubKey))
	privKeyb := []byte(base64.StdEncoding.EncodeToString(privKey))

	// Ask for password
	fmt.Print("Enter password for encrypting the private key: ")
	password, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		err = fmt.Errorf("failed reading password: %v", err)
		return
	}

	// Generate salt
	salt := make([]byte, saltLength)
	_, err = rand.Read(salt)
	if err != nil {
		logError("Failed to generate random salt", err)
	}

	// Get a key derived from the password
	encryptionKey := pbkdf2.Key(password, salt, iterations, keyLength, sha256.New)

	// New cipher
	cipher, err := chacha20poly1305.New(encryptionKey)
	if err != nil {
		logError("Failed to create new cipher", err)
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	_, err = rand.Read(nonce)

	// Encrypt key
	encryptedPrivateKey := cipher.Seal(nonce, nonce, privKeyb, nil)

	// Add salt to cipher text
	encryptedPrivateKeyFile := append(salt, encryptedPrivateKey...)

	// Write private key to user selected files
	err = os.WriteFile(privateKeyFilePath, encryptedPrivateKeyFile, 0600)
	if err != nil {
		logError("Failed to write private key", err)
	}

	// Write public key to user selected files
	err = os.WriteFile(publicKeyFilePath, pubKeyb, 0600)
	if err != nil {
		logError("Failed to write private key", err)
	}

	fmt.Printf("Complete: ed25519 keys generated. Private key at '%s'. Public key at '%s'\n", privateKeyFilePath, publicKeyFilePath)
}

func signFile(sourceFilePath string, privateKeyFilePath string) {
	// Reorganize ELF sections of source file
	cmd := exec.Command("objcopy", sourceFilePath, sourceFilePath)
	_, err := RunCommand(cmd, nil)
	if err != nil {
		logError("Failed to reorganize ELF sections", err)
	}

	// Load file to be signed
	sourceFile, err := os.ReadFile(sourceFilePath)
	if len(sourceFile) == 0 {
		logError("Failed to read source file (empty)", err)
	}

	// Load private key
	privKeyFile, err := os.ReadFile(privateKeyFilePath)
	if len(privKeyFile) == 0 {
		logError("Failed to read private key file (empty)", err)
	}

	// Ask for password
	fmt.Print("Enter password for encrypting the private key: ")
	password, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		err = fmt.Errorf("failed reading password: %v", err)
		return
	}

	// Get salt and key separated
	salt := privKeyFile[:saltLength]
	encryptedPrivateKey := privKeyFile[saltLength:]

	// Derive key
	encryptionKey := pbkdf2.Key(password, salt, iterations, keyLength, sha256.New)

	// New cipher
	cipher, err := chacha20poly1305.New(encryptionKey)
	if err != nil {
		logError("Failed to create new cipher", err)
	}

	// Decrypt the key
	nonce, encryptedPrivateKey := encryptedPrivateKey[:chacha20poly1305.NonceSize], encryptedPrivateKey[chacha20poly1305.NonceSize:]
	privKeyb, err := cipher.Open(nil, nonce, encryptedPrivateKey, nil)

	// Decode key
	privateKey, err := base64.StdEncoding.DecodeString(string(privKeyb))
	if len(privateKey) == 0 {
		logError("Failed to decode private key (empty)", err)
	}

	// Generate Signature
	signature := ed25519.Sign(privateKey, sourceFile)

	// Convert signature to base64 then to byte array
	sig := []byte(base64.StdEncoding.EncodeToString(signature))

	// Add sigdata section to file
	cmd = exec.Command("objcopy", "--add-section", "sigdata=/dev/stdin", sourceFilePath, sourceFilePath)
	_, err = RunCommand(cmd, sig)
	if err != nil {
		logError("Failed to add signature data to file", err)
	}

	fmt.Printf("Complete: ed25519 signature added to 'sigdata' section of file '%s' using private key at '%s'\n", sourceFilePath, privateKeyFilePath)
}

func verifyFile(sourceFilePath string, publicKeyFilePath string) {
	// Load pub key
	pubKeyFile, err := os.ReadFile(publicKeyFilePath)
	if len(pubKeyFile) == 0 {
		logError("Failed to read pub key file (empty)", err)
	}

	// Decode key
	publicKey, err := base64.StdEncoding.DecodeString(string(pubKeyFile))
	if len(publicKey) == 0 {
		logError("Failed to decode public key (empty)", err)
	}

	// Get sigdata section from source file
	command := exec.Command("objcopy", "--dump-section", "sigdata=/dev/stdout", sourceFilePath)
	signatureData, err := RunCommand(command, nil)
	if len(signatureData) == 0 {
		logError("Failed to extract signature data from file (empty)", err)
	}

	// Decode sig
	sig, err := base64.StdEncoding.DecodeString(string(signatureData))
	if len(sig) == 0 {
		logError("Failed to decode signature (empty)", err)
	}
	signature := []byte(sig)

	// Remove sigdata section from source file
	command = exec.Command("objcopy", "--remove-section=sigdata", sourceFilePath, sourceFilePath)
	_, err = RunCommand(command, nil)
	if err != nil {
		logError("Failed to remove signature data from source file", err)
	}

	// Load file to be signed
	sourceFile, err := os.ReadFile(sourceFilePath)
	if len(sourceFile) == 0 {
		logError("Failed to read source file (empty)", err)
	}

	// Verify sig
	IsFileValid := ed25519.Verify(publicKey, sourceFile, signature)
	if IsFileValid {
		fmt.Printf("Complete: File '%s' is VALID based on the ed25519 public key at '%s'\n", sourceFilePath, publicKeyFilePath)
	} else if IsFileValid == false {
		fmt.Printf("Complete: File '%s' is NOT VALID based on the ed25519 public key at '%s'\n", sourceFilePath, publicKeyFilePath)
	} else {
		logError("could not verify", fmt.Errorf("validity bool is neither true or false"))
	}
}

func RunCommand(cmd *exec.Cmd, input []byte) ([]byte, error) {
	// Init command buffers
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Prepare stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	defer stdin.Close()

	// Run the command
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	// Write channel contents to stdin and close input
	_, err = stdin.Write(input)
	if err != nil {
		return nil, err
	}
	stdin.Close()

	// Wait for command to finish
	err = cmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("%v %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}
