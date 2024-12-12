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
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// ###################################
//      EXCEPTION HANDLING
// ###################################

func logError(errorDescription string, errorMessage error) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}

	// Get the current time
	currentTime := time.Now()

	// Format the timestamp
	logTimestamp := currentTime.Format("Jan 01 09:34:56")

	// format error message
	message := fmt.Sprintf("%s Error: %s: %v\n", logTimestamp, errorDescription, errorMessage)

	// Write to log file
	logFile, _ := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	logFile.WriteString(message)

	// Exit with error
	os.Exit(1)
}

// ###################################
//      GLOBAL VARS
// ###################################

var logFilePath string

const defaultLogFilePath string = "/tmp/scmpd_updater.log"
const codeSigningPublicKey string = "eyoi8/fvhtbZiBBxcpseG44hKg2xA9r/IWp8TzKFyaM="
const progVersion string = "v1.2.0"

// ###################################
//      START HERE
// ###################################

func main() {
	// Program Argument Variables
	var sourceFilePath string
	var versionFlagExists bool
	var versionNumberFlagExists bool

	// Read Program Arguments
	flag.StringVar(&sourceFilePath, "src", "", "File path to the source executable for update")
	flag.StringVar(&logFilePath, "logfile", "", "Log file path")
	flag.BoolVar(&versionFlagExists, "V", false, "Print Version Information")
	flag.BoolVar(&versionNumberFlagExists, "v", false, "")
	flag.Parse()

	// Meta info print out
	if versionFlagExists {
		fmt.Printf("Deployer Updater %s compiled using %s(%s) on %s architecture %s\n", progVersion, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("Packages: runtime strings io encoding/base64 flag os/signal fmt time os/exec syscall os bytes path/filepath crypto/ed25519\n")
		return
	} else if versionNumberFlagExists {
		fmt.Println(progVersion)
		return
	}

	// Disallow updating as root/sudo
	if os.Geteuid() == 0 {
		fmt.Printf("Refusing to run updater as root or with sudo.\n")
		os.Exit(1)
	}

	// Set global log file path
	if logFilePath == "" {
		logFilePath = defaultLogFilePath
	}

	// Prevent external from interfering with execution of update
	// Must accept SIGTERM so systemd restart can complete
	signal.Ignore(syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)

	// Validate signature of received update file
	err := ValidateUpdateFile(sourceFilePath)
	logError("Aborting Update", err)

	// Read in sudo password from stdin for passthrough to sudo
	SudoPassword, err := io.ReadAll(os.Stdin)
	logError("Aborting Update: failed to read stdin password", err)

	// Use the parent PID of self as the target for the update
	tgtProcessID := os.Getppid()

	// Format the process exe symlink
	exeSymLink := fmt.Sprintf("/proc/%d/exe", tgtProcessID)

	// Get destination file name from exe sym link
	destinationFilePath, err := os.Readlink(exeSymLink)
	logError("Aborting Update: unable to find destination executable path", err)

	// Ensure a valid path is present for destinationFilePath
	if len(destinationFilePath) == 0 {
		logError("Aborting Update: invalid destination executable path", fmt.Errorf("/proc/%d/exe does not link to anywhere", tgtProcessID))
	}
	if strings.Contains(destinationFilePath, " (deleted)") {
		// Attempt to use a 'deleted' executable file path anyways
		destinationFilePath = strings.TrimSuffix(destinationFilePath, " (deleted)")
	}

	// Get file meta of existing executable
	exeInfo, err := os.Stat(destinationFilePath)
	logError("Aborting Update: unable to retrieve file permissions from running executable file", err)

	// Get permissions and owner for existing executable to apply to updated executable
	stat := exeInfo.Sys().(*syscall.Stat_t)
	ownergroup := fmt.Sprintf("%v:%v", stat.Uid, stat.Gid)
	permissionBits := fmt.Sprintf("%o", exeInfo.Mode())

	// Set permissions from running executable file to the new executable file
	command := exec.Command("sudo", "-S", "chmod", permissionBits, sourceFilePath)
	_, err = RunCommand(command, SudoPassword)
	logError("Aborting Update: unable to change updated executable permissions", err)

	// Set owner/group from running executable file to the new executable file
	command = exec.Command("sudo", "-S", "chown", ownergroup, sourceFilePath)
	_, err = RunCommand(command, SudoPassword)
	logError("Aborting Update: unable to change updated executable owner/group", err)

	// Delete running executable file
	command = exec.Command("sudo", "-S", "rm", destinationFilePath)
	_, err = RunCommand(command, SudoPassword)
	logError("Aborting Update: unable to remove existing executable file for update", err)

	// Move source file to running executable (now deleted) file path
	command = exec.Command("sudo", "-S", "mv", sourceFilePath, destinationFilePath)
	_, err = RunCommand(command, SudoPassword)
	logError("Updated Failed: unable to move updated file to destination", err)

	// Stop parent process - rely on systemd auto-restart to start the service back up
	// Expecting that deployer will catch the SIGTERM and wait for this program to exit
	parentProcess, _ := os.FindProcess(tgtProcessID)
	err = parentProcess.Signal(syscall.SIGTERM)
	logError("Updated Failed: unable to stop deployer process", err)

	// Send stdout back to deployer to go back to controller to signal update succeeded
	fmt.Print("Deployer update successful")
}

// Extracts, validates, and removes the embedded signature of an ELF binary
// Uses built-in code signing public key to validate embedded signature
func ValidateUpdateFile(sourceFilePath string) (err error) {
	// Get sigdata section from source file
	command := exec.Command("objcopy", "--dump-section", "sigdata=/dev/stdout", sourceFilePath)
	signatureData, err := command.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("unable to extract signature data from source file: %v", err)
		return
	}

	// Decode sig
	sig, err := base64.StdEncoding.DecodeString(string(signatureData))
	if err != nil {
		err = fmt.Errorf("unable to decode signature from source file: %v", err)
		return
	}
	signature := []byte(sig)

	// Remove sigdata section from source file
	command = exec.Command("objcopy", "--remove-section=sigdata", sourceFilePath, sourceFilePath)
	_, err = command.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("unable to remove signature data from source file: %v", err)
		return
	}

	// Load file to check signature against
	sourceFile, err := os.ReadFile(sourceFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read source file (empty): %v", err)
		return
	}

	// Decode code signing pubkey - dont care about error, input is fixed
	publicKey, _ := base64.StdEncoding.DecodeString(codeSigningPublicKey)

	// Verify signature
	ValidSignature := ed25519.Verify(publicKey, sourceFile, signature)
	if !ValidSignature {
		err = fmt.Errorf("source file signature is NOT valid")
		return
	}

	// Valid - continue with update
	return
}

// Run a system program with stdin and retrieve stdout and stderr
func RunCommand(cmd *exec.Cmd, input []byte) (standardoutput []byte, err error) {
	// Init command buffers
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Prepare stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	defer stdin.Close()

	// Run the command
	err = cmd.Start()
	if err != nil {
		return
	}

	// Write channel contents to stdin and close input
	_, err = stdin.Write(input)
	if err != nil {
		return
	}
	stdin.Close()

	// Wait for command to finish
	err = cmd.Wait()
	if err != nil {
		err = fmt.Errorf("%v %s", err, stderr.String())
		return
	}

	standardoutput = stdout.Bytes()
	return
}
