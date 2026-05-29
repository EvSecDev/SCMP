package logctx

import (
	"time"
)

const (
	// Integer for printing increasingly detailed information as program progresses
	//
	//	0 - None: quiet (prints nothing but errors)
	//	1 - Standard: normal progress messages
	//	2 - Progress: more progress messages (no actual data outputted)
	//	3 - Data: shows limited data being processed
	//	4 - FullData: shows full data being processed
	//	5 - Debug: shows extra data during processing (raw bytes)
	VerbosityNone int = iota
	VerbosityStandard
	VerbosityProgress
	VerbosityData
	VerbosityFullData
	VerbosityDebug

	// Context keys
	LoggerKey  CtxKey = "logger"  // Event queue (mostly for variable log verbosity handling)
	LogTagsKey CtxKey = "logtags" // List of tags in order of broad->specific appended/popped at various parts of the program

	// Descriptive names for available severity levels
	FatalLog string = "Fatal"
	ErrorLog string = "Error"
	WarnLog  string = "Warn"
	InfoLog  string = "Info"

	// Namespacing Name Components
	NSTest       string = "Test"
	NSLogger     string = "Logger"
	NSDeploy     string = "Deploy"
	NSParsing    string = "Parsing"
	NSValidation string = "Validation"
	NSExec       string = "Exec"
	NSFiles      string = "LocalFiles"
	NSWeb        string = "Web"
	NSAuth       string = "Auth"
	NSSetup      string = "Setup"
	NSSeed       string = "Seed"
	NSGit        string = "Git"
	NSVault      string = "Vault"
	NSSSH        string = "SSH"
	NSArtifacts  string = "Artifacts"

	// Deduplication
	dedupWindow      = 5 * time.Second
	minRepeats       = 10
	suppressCooldown = 1 * time.Minute

	// Output
	maxOutputWriteFailures int = 12 // Maximum times output write can fail before log event is dropped
)
