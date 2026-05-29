package main

import (
	"flag"
	"fmt"
	"os"
	"scmp/cmd/builder/build"
)

func main() {
	const usage = `Program Builder
  Tests, Lints, Compiles, and Automates Building and Releasing

Options:
  -b, --build                    Build the program using defaults
  -a, --arch <arch>              Architecture of compiled binary (amd64, arm64) [default: amd64]
  -o, --os <os>                  Which operating system to build for (linux, freebsd) [default: linux]
  -n, --skip-tests               Skip all tests
  -f, --full-tests               Run intense tests (-race -bench)
  -C, --pre-commit               Pre-commit mode - enables final preparations before a commit
  -u, --update                   Update go packages for program
  -D, --dep-tree                 Print dependency tree
  -L, --check-licenses           Print licenses of all dependencies and their validity with the config
  -p, --prepare-release          Prepare release notes and attachments
  -P, --public-release <version> Publish release to github
  -h, --help                     Print this help menu
`
	var cliOpts build.Options
	flag.BoolVar(&cliOpts.Build, "b", false, "")
	flag.BoolVar(&cliOpts.Build, "build", false, "")
	flag.StringVar(&cliOpts.Architecture, "a", "amd64", "")
	flag.StringVar(&cliOpts.Architecture, "arch", "amd64", "")
	flag.StringVar(&cliOpts.OperatingSystem, "o", "linux", "")
	flag.StringVar(&cliOpts.OperatingSystem, "os", "linux", "")
	flag.BoolVar(&cliOpts.SkipTests, "n", false, "")
	flag.BoolVar(&cliOpts.SkipTests, "skip-tests", false, "")
	flag.BoolVar(&cliOpts.FullTests, "f", false, "")
	flag.BoolVar(&cliOpts.FullTests, "full-tests", false, "")
	flag.BoolVar(&cliOpts.PreCommitMode, "C", false, "")
	flag.BoolVar(&cliOpts.PreCommitMode, "pre-commit", false, "")
	flag.BoolVar(&cliOpts.Updatepackages, "u", false, "")
	flag.BoolVar(&cliOpts.Updatepackages, "update", false, "")
	flag.BoolVar(&cliOpts.PrintDepTree, "D", false, "")
	flag.BoolVar(&cliOpts.PrintDepTree, "dep-tree", false, "")
	flag.BoolVar(&cliOpts.PrintLicenses, "L", false, "")
	flag.BoolVar(&cliOpts.PrintLicenses, "check-licenses", false, "")
	flag.BoolVar(&cliOpts.PrepareRelease, "p", false, "")
	flag.BoolVar(&cliOpts.PrepareRelease, "prepare-release", false, "")
	flag.StringVar(&cliOpts.PublishVersion, "P", "", "")
	flag.StringVar(&cliOpts.PublishVersion, "public-release", "", "")

	flag.Usage = func() { fmt.Printf("Usage: %s [OPTIONS]...\n%s", os.Args[0], usage) }
	flag.Parse()

	exitCode := cliOpts.RunBuilder()
	os.Exit(exitCode)
}
