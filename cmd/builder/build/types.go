package build

// User specified options from CLI
type Options struct {
	Build           bool
	Architecture    string
	OperatingSystem string
	SkipTests       bool
	FullTests       bool
	PreCommitMode   bool
	Updatepackages  bool
	PrintDepTree    bool
	PrintLicenses   bool
	PrepareRelease  bool
	PublishVersion  string
}

// Build configuration from file in repository
type config struct {
	ProgramOutputName            string          `json:"binaryShortName"`
	ProgramLongPrefix            string          `json:"binaryLongNamePrefix"`
	ReadmeHelpMenuStartDelimiter string          `json:"readmeHelpMenuStartDelimiter"`
	RemoteGitRepo                string          `json:"remoteGitRepo"`
	RemoteGitUsername            string          `json:"remoteGitUsername"`
	License                      licenseSettings `json:"licenses"`
}

type licenseSettings struct {
	Permitted  []string `json:"permitted"`
	Disallowed []string `json:"disallowed"`
}

// Data container for build-related info
type context struct {
	cliOpts        Options
	cfg            config
	repositoryRoot string
}

// Go mod download JSON
type goDownloadJSON struct {
	Path     string `json:"Path"`
	Main     bool   `json:"Main,omitempty"`
	Version  string `json:"Version"`
	Info     string `json:"Info"`
	GoMod    string `json:"GoMod"`
	Zip      string `json:"Zip"`
	Dir      string `json:"Dir"`
	Sum      string `json:"Sum"`
	GoModSum string `json:"GoModSum"`
}
