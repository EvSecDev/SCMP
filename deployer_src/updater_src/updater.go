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

// Integer for printing increasingly detailed information as program progresses
//
//	0 - None: quiet (prints nothing but errors)
//	1 - Standard: normal progress messages
//	2 - Progress: more progress messages (no actual data outputted)
//	3 - Data: shows limited data being processed
//	4 - FullData: shows full data being processed
var globalVerbosityLevel int

// Descriptive Names for available verbosity levels
const (
	VerbosityNone int = iota
	VerbosityStandard
	VerbosityProgress
	VerbosityData
	VerbosityFullData
)

var logFilePath string
var dryRunRequested bool

const codeSigningPublicKey string = "eyoi8/fvhtbZiBBxcpseG44hKg2xA9r/IWp8TzKFyaM="
const progVersion string = "v1.3.0"
const usage = `
Options:
    -s, --src                File path to the source executable for update
    -u, --update-updater     Use the source executable to update this program
    -l, --logfile            Log file path [default: /tmp/scmpd_updater.log]
    -T, --dry-run            Runs through all actions and checks for error before starting server
    -v, --verbosity <0...4>  Increase details and frequency of progress messages (Higher number = more verbose) [default: 1]
    -h, --help               Show this help menu
    -V, --version            Show version and packages
        --versionid          Show only version number
`

// ###################################
//      START HERE
// ###################################

// Print message to stdout
// Message will only print if the global verbosity level is equal to or smaller than requiredVerbosityLevel
// Can directly take variables as values to print just like fmt.Printf
func printMessage(requiredVerbosityLevel int, message string, vars ...interface{}) {
	// No output for verbosity level 0
	if globalVerbosityLevel == 0 {
		return
	}

	// Add timestamps to verbosity levels 2 and up
	if globalVerbosityLevel >= 2 {
		currentTime := time.Now()
		timestamp := currentTime.Format("15:04:05.000000")
		message = timestamp + ": " + message
	}

	// Required stdout message verbosity level is equal to or less than global verbosity level
	if requiredVerbosityLevel <= globalVerbosityLevel {
		fmt.Printf(message, vars...)
	}
}

func main() {
	// Program Argument Variables
	var sourceFilePath string
	var updateSelf bool
	var versionFlagExists bool
	var versionNumberFlagExists bool

	// Read Program Arguments
	flag.StringVar(&sourceFilePath, "s", "", "")
	flag.StringVar(&sourceFilePath, "src", "", "")
	flag.BoolVar(&updateSelf, "u", false, "")
	flag.BoolVar(&updateSelf, "update-updater", false, "")
	flag.StringVar(&logFilePath, "l", "/tmp/scmpd_updater.log", "")
	flag.StringVar(&logFilePath, "logfile", "/tmp/scmpd_updater.log", "")
	flag.BoolVar(&dryRunRequested, "T", false, "")
	flag.BoolVar(&dryRunRequested, "dry-run", false, "")
	flag.IntVar(&globalVerbosityLevel, "v", 1, "")
	flag.IntVar(&globalVerbosityLevel, "verbosity", 1, "")
	flag.BoolVar(&versionFlagExists, "V", false, "")
	flag.BoolVar(&versionFlagExists, "version", false, "")
	flag.BoolVar(&versionNumberFlagExists, "versionid", false, "")

	// Custom help menu
	flag.Usage = func() { fmt.Printf("Usage: %s [OPTIONS]...\n%s", os.Args[0], usage) }
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
		printMessage(VerbosityStandard, "Refusing to run updater as root or with sudo.\n")
		os.Exit(1)
	}

	printMessage(VerbosityProgress, "Ignoring syscalls\n")

	// Prevent external from interfering with execution of update
	// Must accept SIGTERM so systemd restart can complete
	signal.Ignore(syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)

	printMessage(VerbosityProgress, "Validating source file signature\n")

	// Validate signature of received update file
	err := ValidateUpdateFile(sourceFilePath)
	logError("Aborting Update", err)

	printMessage(VerbosityProgress, "Reading password from standard in\n")

	// Read in sudo password from stdin for passthrough to sudo
	SudoPassword, err := io.ReadAll(os.Stdin)
	logError("Aborting Update: failed to read stdin password", err)

	// Use process ID of self if updating self or parent process ID if not
	var tgtProcessID int
	if updateSelf {
		printMessage(VerbosityProgress, "Using process PID as target process for update\n")
		tgtProcessID = os.Getpid()
	} else {
		printMessage(VerbosityProgress, "Getting parent process PID as target process for update\n")
		tgtProcessID = os.Getppid()
	}

	// Format the process exe symlink
	exeSymLink := fmt.Sprintf("/proc/%d/exe", tgtProcessID)

	printMessage(VerbosityProgress, "Retrieving file path to the target process executable\n")

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

	printMessage(VerbosityProgress, "Getting permissions/ownership for source file\n")

	// Get file meta of existing executable
	exeInfo, err := os.Stat(destinationFilePath)
	logError("Aborting Update: unable to retrieve file permissions from running executable file", err)

	// Get permissions and owner for existing executable to apply to updated executable
	stat := exeInfo.Sys().(*syscall.Stat_t)
	ownergroup := fmt.Sprintf("%d:%d", stat.Uid, stat.Gid)
	permissionBits := fmt.Sprintf("%o", exeInfo.Mode())

	printMessage(VerbosityProgress, "Setting permissions/ownership on source file to match destination executable\n")

	// Set permissions from running executable file to the new executable file
	command := exec.Command("sudo", "-S", "chmod", permissionBits, sourceFilePath)
	_, err = RunCommand(command, SudoPassword)
	printMessage(VerbosityData, "Running command %s\n", command)
	logError("Aborting Update: unable to change updated executable permissions", err)

	// Set owner/group from running executable file to the new executable file
	command = exec.Command("sudo", "-S", "chown", ownergroup, sourceFilePath)
	_, err = RunCommand(command, SudoPassword)
	printMessage(VerbosityData, "Running command %s\n", command)
	logError("Aborting Update: unable to change updated executable owner/group", err)

	printMessage(VerbosityProgress, "Removing executable of running target process\n")

	// Delete running executable file
	command = exec.Command("sudo", "-S", "rm", destinationFilePath)
	_, err = RunCommand(command, SudoPassword)
	printMessage(VerbosityData, "Running command %s\n", command)
	logError("Aborting Update: unable to remove existing executable file for update", err)

	printMessage(VerbosityProgress, "Moving source file to target process executable path\n")

	// Move source file to running executable (now deleted) file path
	command = exec.Command("sudo", "-S", "mv", sourceFilePath, destinationFilePath)
	_, err = RunCommand(command, SudoPassword)
	printMessage(VerbosityData, "Running command %s\n", command)
	logError("Updated Failed: unable to move updated file to destination", err)

	printMessage(VerbosityProgress, "Sending SIGTERM to target process\n")

	// Stop parent process (if target is daemon, rely on systemd auto-restart to start the service back up)
	// Expecting that the target will catch the SIGTERM and wait for this program to exit
	tgtProcess, _ := os.FindProcess(tgtProcessID)
	err = tgtProcess.Signal(syscall.SIGTERM)
	logError("Updated Failed: unable to stop update target process", err)

	// Send stdout back to deployer to go back to controller to signal update succeeded
	printMessage(VerbosityStandard, "Update successful")
	printMessage(VerbosityProgress, "Exiting\n")
}

// Extracts, validates, and removes the embedded signature of an ELF binary
// Uses built-in code signing public key to validate embedded signature
func ValidateUpdateFile(sourceFilePath string) (err error) {
	printMessage(VerbosityProgress, "Extracting signature secture from ELF source file\n")

	// Get sigdata section from source file
	command := exec.Command("objcopy", "--dump-section", "sigdata=/dev/stdout", sourceFilePath)
	signatureData, err := command.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("unable to extract signature data from source file: %v", err)
		return
	}

	printMessage(VerbosityProgress, "Decoding source file signature\n")

	// Decode sig
	sig, err := base64.StdEncoding.DecodeString(string(signatureData))
	if err != nil {
		err = fmt.Errorf("unable to decode signature from source file: %v", err)
		return
	}
	signature := []byte(sig)

	printMessage(VerbosityProgress, "Removing signature data from source file\n")

	// Remove sigdata section from source file
	command = exec.Command("objcopy", "--remove-section=sigdata", sourceFilePath, sourceFilePath)
	_, err = command.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("unable to remove signature data from source file: %v", err)
		return
	}

	printMessage(VerbosityProgress, "Reading source file for signature check\n")

	// Load file to check signature against
	sourceFile, err := os.ReadFile(sourceFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read source file (empty): %v", err)
		return
	}

	// Decode code signing pubkey - dont care about error, input is fixed
	publicKey, _ := base64.StdEncoding.DecodeString(codeSigningPublicKey)

	printMessage(VerbosityProgress, "Verifying source file signature\n")

	// Verify signature
	ValidSignature := ed25519.Verify(publicKey, sourceFile, signature)
	if !ValidSignature {
		err = fmt.Errorf("source file signature is NOT valid")
		return
	}

	printMessage(VerbosityProgress, "Source file signature is valid\n")

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

	printMessage(VerbosityData, "  Command start\n")

	// Run the command
	err = cmd.Start()
	if err != nil {
		return
	}

	printMessage(VerbosityData, "  Writing stdin to command\n")

	// Write channel contents to stdin and close input
	_, err = stdin.Write(input)
	if err != nil {
		return
	}
	stdin.Close()

	printMessage(VerbosityData, "  Waiting for command to finish\n")

	// Wait for command to finish
	err = cmd.Wait()
	if err != nil {
		err = fmt.Errorf("%v %s", err, stderr.String())
		return
	}

	printMessage(VerbosityData, "  Retrieving stdout from command\n")

	standardoutput = stdout.Bytes()
	return
}
