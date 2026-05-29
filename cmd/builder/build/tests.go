package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"strconv"
	"strings"
)

// Test coverage persistent storage
type testCoverageStore struct {
	Tests map[string]testCoverageTracker `json:"tests"`
}

type testCoverageTracker struct {
	LastCommitPercent float64 `json:"lastCommitPct"`
	LastBuildPercent  float64 `json:"lastBuildPct"`
}

func runTests(ctx *context) (err error) {
	hasTests := false
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if strings.HasSuffix(path, "_test.go") {
			hasTests = true
			return io.EOF
		}
		return nil
	})
	if !hasTests {
		return
	}

	testArgs := []string{
		"-timeout=4m",
	}
	if ctx.cliOpts.FullTests {
		testArgs = append(testArgs, "-race", "-bench=.")
	}

	// Unit Tests (run by directory)
	err = runUnitTests(ctx, testArgs)
	if err != nil {
		return
	}

	if !ctx.cliOpts.FullTests {
		return
	}

	return
}

func runUnitTests(ctx *context, testArgs []string) (err error) {
	printInfo(0, "Running unit tests...")

	type testInfo struct {
		absolutePath string
		testCoverage string
	}
	tests := map[string]testInfo{
		"CLI": {
			absolutePath: filepath.Join(ctx.repositoryRoot, "cli"),
		},
		"Core": {
			absolutePath: filepath.Join(ctx.repositoryRoot, "core"),
		},
		"Internal": {
			absolutePath: filepath.Join(ctx.repositoryRoot, "internal"),
		},
		"Setup": {
			absolutePath: filepath.Join(ctx.repositoryRoot, "setup"),
		},
		"Web": {
			absolutePath: filepath.Join(ctx.repositoryRoot, "web"),
		},
	}

	// Run the tests
	for testName, testInfo := range tests {
		coverProfileOut := filepath.Join(ctx.repositoryRoot, ".coverprofile_"+testName+".out")
		defer func() {
			_ = os.Remove(coverProfileOut)
		}()

		args := []string{"test", "-C", testInfo.absolutePath}
		args = append(args, testArgs...)
		args = append(args, "-coverprofile="+coverProfileOut)
		args = append(args, "./...")

		cmd := exec.Command("go", args...)
		err = helpers.RunTestCommand(*cmd)
		if err != nil {
			return
		}

		// Get total test coverage
		var coveragePercent string
		coveragePercent, err = extractTestCoverage(coverProfileOut)
		if err != nil {
			return
		}

		testInfo.testCoverage = coveragePercent
		tests[testName] = testInfo
		_ = os.Remove(coverProfileOut)
	}

	printSuccess(0, "Done")
	printInfo(0, "Unit Test Coverage Totals:")

	// Show coverages and diffs
	testCoverages, err := loadCoverageStore(ctx)
	if err != nil {
		return
	}
	for testName, testInfo := range tests {
		covLine := testCoverages.formatCoverageInfo(ctx, testName, testInfo.testCoverage)
		printInfo(4, covLine)
	}
	err = testCoverages.saveStore(ctx)
	if err != nil {
		return
	}

	printSuccess(0, "Done")
	return
}

// Retrieves test coverage percentage from the cover profile file using go tool
func extractTestCoverage(profileFile string) (coverPercent string, err error) {
	cmd := exec.Command("go", "tool", "cover", "-func="+profileFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("go tool cover: %w: %s", err, string(out))
		return
	}

	lines := bytes.SplitSeq(out, []byte("\n"))
	for line := range lines {
		if bytes.Equal(line, []byte("")) {
			continue
		}
		if !bytes.HasPrefix(line, []byte("total:")) {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) < 3 {
			continue
		}
		coverPercent = string(fields[2])
	}
	if coverPercent == "" {
		err = fmt.Errorf("failed to extract coverage percentage from go tool output: '%s'", string(out))
		return
	}
	if coverPercent == "0.0%" {
		//err = fmt.Errorf("test cover profile has 0%% coverage, this is probably an error")
		return
	}
	return
}

// Loads and parses JSON test coverage file from repo
func loadCoverageStore(ctx *context) (testCoverages *testCoverageStore, err error) {
	testCoverages = &testCoverageStore{}

	mainTestCovStore := filepath.Join(ctx.repositoryRoot, persistentCoverageStore)

	testCovFile, err := os.ReadFile(mainTestCovStore)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("unable to access test coverage store file: %w", err)
		return
	}
	if err != nil && errors.Is(err, os.ErrNotExist) {
		testCoverages = &testCoverageStore{
			Tests: make(map[string]testCoverageTracker),
		}
	} else {
		err = json.Unmarshal(testCovFile, testCoverages)
		if err != nil {
			err = fmt.Errorf("failed to parse test coverage JSON: %w", err)
			return
		}

		if testCoverages.Tests == nil {
			testCoverages.Tests = make(map[string]testCoverageTracker)
		}
	}

	return
}

// Writes updated test coverage JSON to file
func (testCoverages *testCoverageStore) saveStore(ctx *context) (err error) {
	mainTestCovStore := filepath.Join(ctx.repositoryRoot, persistentCoverageStore)

	newTestCovFile, err := json.MarshalIndent(testCoverages, "", "  ")
	if err != nil {
		err = fmt.Errorf("failed to marshal new test coverages: %w", err)
		return
	}
	err = os.WriteFile(mainTestCovStore, newTestCovFile, 0600)
	if err != nil {
		err = fmt.Errorf("failed to write new test coverage file: %w", err)
		return
	}

	return
}

// Creates current test coverage and coverage diff string
func (testCoverages *testCoverageStore) formatCoverageInfo(ctx *context, testName string, currentCoveragePercent string) (covInfo string) {
	currentCoverage, err := strconv.ParseFloat(strings.TrimRight(currentCoveragePercent, "%"), 64)
	if err != nil {
		currentCoverage = 0
	}

	// Evaluate change
	var diffSinceCommit, diffSinceBuild float64
	testCovInfo, testWasRecorded := testCoverages.Tests[testName]
	if testWasRecorded {
		// Commit change
		diffSinceCommit = currentCoverage - testCovInfo.LastCommitPercent
		if math.Abs(diffSinceCommit) < 0.1 {
			// Discard very small changes
			diffSinceCommit = 0
		}
		if testCovInfo.LastCommitPercent == 0 {
			// Discard unrecorded percents in json
			diffSinceCommit = 0
		}

		// Build change
		diffSinceBuild = currentCoverage - testCovInfo.LastBuildPercent
		if math.Abs(diffSinceBuild) < 0.1 {
			// Discard very small changes
			diffSinceBuild = 0
		}
		if testCovInfo.LastBuildPercent == 0 {
			// Discard unrecorded percents in json
			diffSinceBuild = 0
		}
	}

	// Build diff line
	var commitDiff, buildDiff string
	if diffSinceCommit > 0 {
		commitDiff = fmt.Sprintf("%s%+.2f%%%s", colorGreen, diffSinceCommit, noColor)
	} else if diffSinceCommit < 0 {
		commitDiff = fmt.Sprintf("%s%+.2f%%%s", colorRed, diffSinceCommit, noColor)
	} else if diffSinceCommit == 0 {
		commitDiff = fmt.Sprintf("%sno change%s", colorBold, noColor)
	}
	if diffSinceBuild > 0 {
		buildDiff = fmt.Sprintf("%s%+.2f%%%s", colorGreen, diffSinceBuild, noColor)
	} else if diffSinceBuild < 0 {
		buildDiff = fmt.Sprintf("%s%+.2f%%%s", colorRed, diffSinceBuild, noColor)
	} else if diffSinceBuild == 0 {
		buildDiff = fmt.Sprintf("%sno change%s", colorBold, noColor)
	}
	covDiff := fmt.Sprintf("(Coverage Diff Since Last: build=%s; commit=%s)", buildDiff, commitDiff)

	covInfo = fmt.Sprintf("%s Coverage: %s%s%s%s %s",
		testName, colorBold, colorBlue, currentCoveragePercent, noColor, covDiff)

	if !testWasRecorded {
		// Initialize map
		testCoverages.Tests[testName] = testCoverageTracker{}
	}

	// Save coverages back to in-memory json
	testCovInfo.LastBuildPercent = currentCoverage
	if ctx.cliOpts.PreCommitMode {
		testCovInfo.LastCommitPercent = currentCoverage
	}
	testCoverages.Tests[testName] = testCovInfo
	return
}
