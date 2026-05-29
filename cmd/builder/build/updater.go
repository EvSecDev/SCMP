package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"strconv"
	"strings"
)

type versionDiff struct {
	old string
	new string
}

func updateGoPackages() (err error) {
	// Require Clean Repo
	cmd := exec.Command("git", "diff", "--quiet")
	_, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("refusing to update when uncommitted changes exist: unstaged changes exist")
		return
	}
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	_, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("refusing to update when uncommitted changes exist: uncommitted changes exist")
		return
	}

	printInfo(0, "Capturing current module state...")

	tmpDir, err := os.MkdirTemp("", "pkg-updates-"+strconv.Itoa(os.Getpid()))
	if err != nil {
		err = fmt.Errorf("failed creating temporary directory: %w", err)
		return
	}
	defer func() {
		lerr := os.RemoveAll(tmpDir)
		if err == nil && lerr != nil {
			err = fmt.Errorf("failed to clean up temporary directory '%s': %w", tmpDir, err)
		}
	}()

	cmd = exec.Command("go", "list", "-m", "all")
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed to list current go modules: %w: %s", err, string(out))
		return
	}
	beforeModuleList := bytes.Split(out, []byte("\n"))

	printInfo(0, "Updating Go packages...")

	// Run the update
	cmd = exec.Command("go", "get", "-u", "all")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed to update go modules: %w: %s", err, string(out))
		return
	}
	cmd = exec.Command("go", "mod", "verify")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed to verify go modules: %w: %s", err, string(out))
		return
	}
	cmd = exec.Command("go", "mod", "tidy")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed to update go.mod: %w: %s", err, string(out))
		return
	}

	printInfo(0, "Capturing updated module state...")

	cmd = exec.Command("go", "list", "-m", "all")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed to list updated go modules: %w: %s", err, string(out))
		return
	}
	afterModuleList := bytes.Split(out, []byte("\n"))

	printInfo(0, "Detecting changed modules...")

	beforeModules := helpers.DualFieldsIntoMap(beforeModuleList)
	afterModules := helpers.DualFieldsIntoMap(afterModuleList)

	// Extract changed modules (old vs new version)

	changedModules := make(map[string]versionDiff)
	for module, afterVersion := range afterModules {
		beforeVersion, ok := beforeModules[module]
		if !ok {
			// Module not present before update
			beforeVersion = "n/a"
		}
		if afterVersion == beforeVersion {
			// Skip unchanged
			continue
		}
		// Versions are different, record
		changedModules[module] = versionDiff{
			old: beforeVersion,
			new: afterVersion,
		}
	}

	if len(changedModules) == 0 {
		printSuccess(0, "No module changes detected")
		return
	}

	printInfo(0, "\n=========== MODULE SOURCE DIFFS ===========")

	for moduleName, versionDifference := range changedModules {
		err = diffModuleSource(moduleName, versionDifference, tmpDir)
		if err != nil {
			return
		}
	}

	printInfo(0, "===========================================\n")

	var userResponse string
	fmt.Printf("[?] Accept these dependency source code changes? (y/N): ")
	_, err = fmt.Scanln(&userResponse)
	if err != nil {
		err = fmt.Errorf("failed to scan for user input: %w", err)
		return
	}
	userResponse = strings.ToLower(userResponse)

	if userResponse != "y" {
		cmd = exec.Command("git", "reset", "--hard", "HEAD")
		out, err = cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("failed to revert git repository to HEAD after user rejected updated module changes: %w: %s",
				err, string(out))
			return
		}
		err = fmt.Errorf("user did not accept changes: reverted repository state to pre-update")
		return
	}

	printSuccess(0, "Done")
	return
}

func diffModuleSource(moduleName string, versionDifference versionDiff, tmpDir string) (err error) {
	printInfo(0, "\n%s", moduleName)
	printInfo(4, "%s -> %s", versionDifference.old, versionDifference.new)

	moduleSafe := strings.ReplaceAll(moduleName, "/", "_")

	oldShort := helpers.ShortVersion(versionDifference.old)
	newShort := helpers.ShortVersion(versionDifference.new)

	tmpOld := filepath.Join(tmpDir, moduleSafe+"@"+oldShort, "old")
	tmpNew := filepath.Join(tmpDir, moduleSafe+"@"+newShort, "new")

	// Ensure nothing is present at created path
	err = os.RemoveAll(tmpOld)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to remove potentially conflicting source code audit directory '%s': %w", tmpOld, err)
		return
	}
	err = os.RemoveAll(tmpNew)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to remove potentially conflicting source code audit directory '%s': %w", tmpNew, err)
		return
	}

	oldSrc := filepath.Join(tmpOld, "src")
	newSrc := filepath.Join(tmpNew, "src")

	err = os.MkdirAll(oldSrc, 0700)
	if err != nil {
		err = fmt.Errorf("failed to create temp source directory for old module: %w", err)
		return
	}
	err = os.MkdirAll(newSrc, 0700)
	if err != nil {
		err = fmt.Errorf("failed to create temp source directory for new module: %w", err)
		return
	}

	printInfo(4, "Downloading old version...")

	cmd := exec.Command("go", "mod", "download", "-json", moduleName+"@"+versionDifference.old)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed to download module %s version %s: %w: %s",
			moduleName, versionDifference.old, err, string(out))
		return
	}
	var oldDownloadInfo goDownloadJSON
	err = json.Unmarshal(out, &oldDownloadInfo)
	if err != nil {
		err = fmt.Errorf("failed parsing go mod download JSON output: %w", err)
		return
	}
	var totalByteSize int64
	totalByteSize, err = helpers.CopyDirRecursive(oldDownloadInfo.Dir, oldSrc)
	if err != nil {
		err = fmt.Errorf("failed to copy old module %s version %s: %w",
			moduleName, versionDifference.old, err)
		return
	}

	// Limit total size so diff isn't excessively large
	maxSize := 100
	megabyteSize := int(totalByteSize / 1_000_000)
	if megabyteSize > maxSize {
		printWarn(4, "Skipping large module directory %s (%s): directory larger than %dMB", moduleName, versionDifference.old, maxSize)
		return
	}

	printInfo(4, "Downloading new version...")

	cmd = exec.Command("go", "mod", "download", "-json", moduleName+"@"+versionDifference.new)
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed to download module %s version %s: %w: %s",
			moduleName, versionDifference.new, err, string(out))
		return
	}
	var newDownloadInfo goDownloadJSON
	err = json.Unmarshal(out, &newDownloadInfo)
	if err != nil {
		err = fmt.Errorf("failed parsing go mod download JSON output: %w", err)
		return
	}

	_, err = helpers.CopyDirRecursive(newDownloadInfo.Dir, newSrc)
	if err != nil {
		err = fmt.Errorf("failed to copy new module %s version %s: %w",
			moduleName, versionDifference.new, err)
		return
	}

	err = os.MkdirAll(filepath.Join(tmpOld, "filtered"), 0700)
	if err != nil {
		err = fmt.Errorf("failed to create temp filtering directory for old module: %w", err)
		return
	}
	err = os.MkdirAll(filepath.Join(tmpNew, "filtered"), 0700)
	if err != nil {
		err = fmt.Errorf("failed to create temp filtering directory for new module: %w", err)
		return
	}

	printInfo(4, "Preparing filtered trees...")

	_, err = helpers.CopyDirRecursiveWithExclude(oldSrc, tmpOld+"/filtered", []string{"_test.go", "testdata/"})
	if err != nil {
		err = fmt.Errorf("failed to copy old source to temp dir for module %s version %s: %w",
			moduleName, versionDifference.old, err)
		return
	}
	_, err = helpers.CopyDirRecursiveWithExclude(newSrc, tmpNew+"/filtered", []string{"_test.go", "testdata/"})
	if err != nil {
		err = fmt.Errorf("failed to copy new source to temp dir for module %s version %s: %w",
			moduleName, versionDifference.old, err)
		return
	}

	printInfo(4, "Diff (excluding tests)...")

	cmd = exec.Command("git", "-c", "color.ui=always", "diff", "--no-index", tmpOld+"/filtered", tmpNew+"/filtered")
	out, err = cmd.CombinedOutput()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			if exitErr.ExitCode() > 1 {
				err = fmt.Errorf("failed to generate source diff for module %s version %s: %w: %s",
					moduleName, versionDifference.old, err, string(out))
				return
			}
		}
	}
	diff := out

	diffLines := bytes.Split(diff, []byte("\n"))
	if len(diffLines) <= 20 {
		fmt.Printf("%s\n", string(diff))
	} else {
		cmd := exec.Command("less", "-R")

		// Sending entire diff to less
		cmd.Stdin = bytes.NewReader(diff)

		// attach to terminal
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			err = fmt.Errorf("failed to run less on version diff for module %s: %w: %s",
				moduleName, err, string(out))
			return
		}
	}

	// Cleanup
	err = os.RemoveAll(tmpOld)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to cleanup old source code audit directory: %w", err)
		return
	}
	err = os.RemoveAll(tmpNew)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to cleanup new source code audit directory: %w", err)
		return
	}
	return
}
