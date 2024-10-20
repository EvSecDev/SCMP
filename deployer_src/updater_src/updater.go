// updater
package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// ###################################
//      EXCEPTION HANDLING
// ###################################

func logError(errorDescription string, errorMessage error, FatalError bool) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}

	// format error message
	message := fmt.Sprintf("%s: %v\n", errorDescription, errorMessage)

	// Write to log file
	logFile, _ := os.OpenFile("/tmp/updater.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	logFile.WriteString(message)

	// Exit if requested
	if FatalError {
		os.Exit(1)
	}
}

// ###################################
//      START HERE
// ###################################

func main() {
	progVersion := "v1.0.0"

	// Parse Arguments
	var sourceFilePath string
	var versionFlagExists bool
	flag.StringVar(&sourceFilePath, "src", "", "File path to the source executable for update")
	flag.BoolVar(&versionFlagExists, "V", false, "Print Version Information")
	flag.Parse()

	// Meta info print out
	if versionFlagExists {
		fmt.Printf("Deployer Updater %s compiled using %s(%s) on %s architecture %s\n", progVersion, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Packages: runtime flag bytes fmt io os os/exec encoding/base64 crypto/ed25519\n")
		os.Exit(0)
	}

	// Disallow updating as root/sudo
	if os.Geteuid() == 0 {
		fmt.Printf("Refusing to run updater as root or with sudo.\n")
		os.Exit(1)
	}

	// Get sigdata section from source file
	command := exec.Command("objcopy", "--dump-section", "sigdata=/dev/stdout", sourceFilePath)
	signatureData, err := RunCommand(command, nil)
	logError("Failed to extract signature data from source file", err, true)

	// Decode sig
	sig, err := base64.StdEncoding.DecodeString(string(signatureData))
	logError("Failed to decode signature from source file", err, true)
	signature := []byte(sig)

	// Remove sigdata section from source file
	command = exec.Command("objcopy", "--remove-section=sigdata", sourceFilePath, sourceFilePath)
	_, err = RunCommand(command, nil)
	logError("Failed to remove signature data from source file", err, true)

	// Load file to check signature against
	sourceFile, err := os.ReadFile(sourceFilePath)
	logError("Failed to read source file (empty)", err, true)

	// Decode pubkey
	publicKey, _ := base64.StdEncoding.DecodeString("cP51e9+eNrisDeJtAHW12JPtcDOjz5WAhx+99KEcbJI=")

	// Verify signature
	ValidSignature := ed25519.Verify(publicKey, sourceFile, signature)
	if !ValidSignature {
		logError("Error: Aborting update", fmt.Errorf("source file signature is NOT valid"), true)
	}

	// Read in sudo password from stdin for passthrough to sudo
	SudoPassword, err := io.ReadAll(os.Stdin)

	// Get the Parent PID
	PPID := os.Getppid()

	// Follow sym link to get executable path
	destinationFilePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", PPID))
	logError("Failed to get destination file name", err, true)

	// Stop parent process - rely on systemd auto-restart to start the service back up
	parentProcess, _ := os.FindProcess(PPID)
	err = parentProcess.Kill()
	logError("Failed to stop deployer process", err, true)

	// Copy source file to destination (to keep owner/perms)
	// using sudo and stdin from controller to write to privileged directory
	command = exec.Command("sudo", "-S", "cp", "--no-preserve=mode,ownership", sourceFilePath, destinationFilePath)
	_, err = RunCommand(command, SudoPassword)
	logError("Failed to copy source file to destination file", err, true)

	// Remove source file
	err = os.Remove(sourceFilePath)
	logError("Failed to remove source file", err, true)
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
