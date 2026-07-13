package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"slices"
	"strings"
)

func preBuildChecks(ctx *context) (err error) {
	err = checkVersioning(ctx)
	if err != nil {
		return
	}
	err = checkDevArtifacts(ctx)
	if err != nil {
		return
	}
	err = runStaticAnalysis(ctx)
	if err != nil {
		return
	}
	return
}

func checkVersioning(ctx *context) (err error) {
	printInfo(0, "Checking versioning...")

	// Get head commit hash
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("git: %w: %s", err, string(out))
		return
	}
	headCommitHash := string(out)

	// Get commit where last release was generated from
	out, err = os.ReadFile(filepath.Join(ctx.repositoryRoot, releaseCommitTracker))
	if err != nil {
		err = fmt.Errorf("failed to get commit hash of last release: %w", err)
		return
	}
	lastReleaseCommitHash := string(bytes.Trim(out, "\n"))

	// Retrieve the program version from the last release commit
	cmd = exec.Command("git", "show", lastReleaseCommitHash+":"+globalConstsFile)
	out, err = cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "exists on disk, but not in") {
			err = nil
			printSuccess(0, "Done")
			return
		} else {
			err = fmt.Errorf("git show: %w: %s", err, string(out))
			return
		}
	}
	lastReleaseVersionNumber, err := helpers.GetProgVersion(out, versionVariableName)
	if err != nil {
		return
	}

	// Get the current version number
	mainConstsFile := filepath.Join(ctx.repositoryRoot, globalConstsFile)
	constsFile, err := os.ReadFile(mainConstsFile)
	if err != nil {
		err = fmt.Errorf("failed to read global consts: %w", err)
		return
	}
	progVersion, err := helpers.GetProgVersion(constsFile, versionVariableName)
	if err != nil {
		return
	}

	// Error if version number hasn't been upped since last commit
	if lastReleaseVersionNumber != "" && lastReleaseVersionNumber == progVersion && headCommitHash != lastReleaseCommitHash {
		err = fmt.Errorf("version number in %s has not been bumped since last commit, failing build", globalConstsFile)
		return
	}

	printSuccess(0, "Done")
	return
}

func checkDevArtifacts(ctx *context) (err error) {
	printInfo(0, "Checking for development artifacts in source code...")

	// Check for any left over debug prints
	found, err := helpers.ScanRepo(ctx.repositoryRoot, false, func(path, line string) (matches bool) {
		matches = strings.Contains(line, "DEBUG")
		return
	})
	if err != nil {
		err = fmt.Errorf("scan repo (DEBUG): %w", err)
		return
	}
	if len(found) > 0 {
		for _, matchedResult := range found {
			fmt.Printf("    %s:%d: %s\n", matchedResult.Path, matchedResult.LineNum, matchedResult.Line)
		}
		printWarn(4, "Debug print found in source code. You might want to remove that before release.")
	}

	// Want functions that have an actual body attached (safety)
	found, err = helpers.ScanRepo(ctx.repositoryRoot, false, func(path, line string) (matches bool) {
		if !strings.HasSuffix(path, ".go") {
			return
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "var") || !strings.Contains(line, "func(") {
			return
		}
		// Func var has a body set
		if strings.Contains(line, ") =") {
			return
		}

		// Tokenize on whitespace
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return
		}

		// Validate identifier (a-zA-Z0-9_)
		name := fields[1]
		for i := 0; i < len(name); i++ {
			c := name[i]
			if c == '_' ||
				c < 'a' || c > 'z' &&
				c < 'A' || c > 'Z' &&
				c < '0' || c > '9' {
				return
			}
		}

		// Third token must start with func(
		if !strings.HasPrefix(fields[2], "func(") {
			return
		}

		matches = true
		return
	})
	if err != nil {
		err = fmt.Errorf("scan repo (func safety): %w", err)
		return
	}
	if len(found) > 0 {
		for _, matchedResult := range found {
			fmt.Printf("    %s:%d: %s\n", matchedResult.Path, matchedResult.LineNum, matchedResult.Line)
		}
		printWarn(4, "Found function variables that do not have an actual function set.")
	}

	// Allowing panic in certain files
	ignorePanicInPaths := []string{"internal/global/misc.go"}

	// Check for any panic usage
	found, err = helpers.ScanRepo(ctx.repositoryRoot, false, func(path, line string) (matches bool) {
		for _, ignorePath := range ignorePanicInPaths {
			if strings.HasSuffix(path, ignorePath) {
				return
			}
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "panic(") {
			matches = true
		}
		return
	})
	if err != nil {
		err = fmt.Errorf("scan repo (panic usage): %w", err)
		return
	}
	if len(found) > 0 {
		for _, matchedResult := range found {
			fmt.Printf("    %s:%d: %s\n", matchedResult.Path, matchedResult.LineNum, matchedResult.Line)
		}
		printWarn(4, "Found panic use. Remember to actually handle the error eventually.")
	}

	printSuccess(0, "Done")
	return
}

func runStaticAnalysis(ctx *context) (err error) {
	printInfo(0, "Running static analysis...")

	// Lint 1
	cmd := exec.Command("staticcheck", "-checks", "all", "./...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Filter out unneeded lines
		lines := bytes.Split(out, []byte("\n"))
		var newLines [][]byte
		for _, line := range lines {
			if slices.Equal(line, []byte("")) {
				continue
			}

			// Disagree on including name in description comments
			if bytes.Contains(line, []byte("comment on exported method")) ||
				bytes.Contains(line, []byte("package comment should be of the form")) ||
				bytes.Contains(line, []byte("comment on exported type")) ||
				bytes.Contains(line, []byte("comment on exported function")) ||
				bytes.Contains(line, []byte("comment on exported var ")) {
				continue
			}
			indentLine := append([]byte("    "), line...)
			newLines = append(newLines, indentLine)
		}

		if len(newLines) > 0 {
			newOut := bytes.Join(newLines, []byte("\n"))
			err = fmt.Errorf("staticcheck: %w\n%s", err, string(newOut))
			return
		}
	}

	// Lint 2
	cmd = exec.Command("go", "vet", "./...")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("go vet: %w\n%s", err, string(out))
		return
	}

	// Lint 3
	cmd = exec.Command("golangci-lint", "run", "--allow-parallel-runners")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("golangci-lint: %w\n%s", err, string(out))
		return
	}

	// Typescript Lint
	err = lintTypeScript(ctx)
	if err != nil {
		err = fmt.Errorf("typescript lint: %w\n%s", err, string(out))
		return
	}

	printSuccess(0, "Done")
	return
}
