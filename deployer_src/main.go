package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"gopkg.in/yaml.v2"
)

// ###################################
//      GLOBAL VARIABLES
// ###################################

// Main Yaml config format
type Config struct {
	UpdaterProgram string `yaml:"UpdaterProgram"`
	SSHServer      struct {
		ListenAddress  string   `yaml:"ListenAddress"`
		ListenPort     string   `yaml:"ListenPort"`
		SSHPrivKeyFile string   `yaml:"SSHPrivKeyFile"`
		AuthorizedUser string   `yaml:"AuthorizedUser"`
		AuthorizedKeys []string `yaml:"AuthorizedKeys"`
	} `yaml:"SSHServer"`
}

// Integer for printing increasingly detailed information as program progresses
//
//	0 - None: quiet (prints nothing but errors)
//	1 - Standard: normal progress messages
//	2 - Progress: more progress messages (no actual data outputted)
//	3 - Data: shows limited data being processed
//	4 - FullData: shows full data being processed
var globalVerbosityLevel int

// User requested test run
var dryRunRequested bool

// Path to updater executable (retrieved from program config)
var UpdaterProgram string

// Descriptive Names for available verbosity levels
const (
	VerbosityNone int = iota
	VerbosityStandard
	VerbosityProgress
	VerbosityData
	VerbosityFullData
)
const connectionRateLimit int = 500 // Sleep time in milliseconds between connection processing (sleeps after the last connection is done)
const progVersion string = "v2.0.0"
const usage = `
Options:
    -c, --config </path/to/yaml>  Path to the configuration file [default: scmpd.yaml]
    -s, --start-server            Start the Deployer SSH Server
    -t, --test-config             Test deployer configuration syntax validity
    -T, --dry-run                 Runs through all actions and checks for error before starting server
    -v, --verbosity <0...4>       Increase details and frequency of progress messages (Higher number = more verbose) [default: 1]
    -h, --help                    Show this help menu
    -V, --version                 Show version and packages
        --versionid               Show only version number

Documentation: <https://github.com/EvSecDev/SCMPusher>
`

// ###################################
//      START
// ###################################

func main() {

	// Program Argument Variables
	var configFilePath string
	var startServerFlagExists bool
	var testConfig bool
	var versionFlagExists bool
	var versionNumberFlagExists bool

	// Read Program Arguments
	flag.StringVar(&configFilePath, "c", "scmpd.yaml", "")
	flag.StringVar(&configFilePath, "config", "scmpd.yaml", "")
	flag.BoolVar(&startServerFlagExists, "s", false, "")
	flag.BoolVar(&startServerFlagExists, "start-server", false, "")
	flag.BoolVar(&testConfig, "t", false, "")
	flag.BoolVar(&testConfig, "test-config", false, "")
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
		fmt.Printf("Deployer %s compiled using %s(%s) on %s architecture %s\n", progVersion, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("Packages: runtime strings io github.com/pkg/sftp encoding/base64 flag os/signal fmt golang.org/x/crypto/ssh os/exec net syscall os bytes encoding/binary gopkg.in/yaml.v2\n")
		return
	}
	if versionNumberFlagExists {
		fmt.Println(progVersion)
		return
	}

	// Grab config file
	yamlConfigFile, err := os.ReadFile(configFilePath)
	logError("Error reading config file", err, true)

	if yamlConfigFile == nil {
		logError("Error reading config file", fmt.Errorf("empty file"), true)
	}

	// Parse all configuration options
	var config Config
	err = yaml.Unmarshal(yamlConfigFile, &config)
	logError("Error unmarshaling config file", err, true)

	// Set global
	UpdaterProgram = config.UpdaterProgram

	// Parse User Choices
	if testConfig {
		// If user wants to test config, just exit once program gets to this point
		// Any config errors will be discovered prior to this point and exit with whatever error happened
		printMessage(VerbosityStandard, "deployer: configuration file %s test is successful\n", configFilePath)
	} else if startServerFlagExists {
		// Server entry point
		RunSSHServer(config, progVersion)
	} else {
		// Exit program without any arguments
		printMessage(VerbosityStandard, "No arguments specified! Use '-h' or '--help' to guide your way.\n")
	}
}
