package build

const (
	// Expected relative paths (to repo root)
	globalConstsFile        string = "internal/global/consts.go" // Source of version string
	persistentCoverageStore string = ".test-coverage.json"       // Storing last test run and last commit run coverage
	releaseCommitTracker    string = ".last_release_commit"      // File that holds the commit hash of last release
	mainREADME              string = "README.md"                 // readme from root of repo

	versionVariableName string = "ProgVersion"

	// Release
	baseAPI   string = "api.github.com"
	uploadAPI string = "uploads.github.com"

	// Output text coloring
	ansiiRed        string = "\033[31m"
	ansiiGreen      string = "\033[32m"
	ansiiYellow     string = "\033[33m"
	ansiiBlue       string = "\033[34m"
	ansiiBold       string = "\033[1m"
	ansiiColorreset string = "\033[0m"
)
