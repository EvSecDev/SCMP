package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func createReleaseStagingDir() (releaseDir string, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	tmpDirOptions := []string{
		filepath.Join(homeDir, "Downloads"),
		filepath.Join(homeDir, ".local", "tmp"),
		"/tmp",
	}

	for _, tmpDirOption := range tmpDirOptions {
		_, err = os.Stat(tmpDirOption)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("failed to check existence of tmp dir: %w", err)
			return
		} else if err != nil && errors.Is(err, os.ErrNotExist) {
			continue
		}
		releaseDir = filepath.Join(tmpDirOption, "releasetemp")
		break
	}
	if releaseDir == "" {
		err = fmt.Errorf("could not identify available temporary directory, cannot continue")
		return
	}

	_, err = os.Stat(releaseDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to check existence of release staging directory: %w", err)
		return
	} else if err == nil {
		// Already created, return early
		return
	}

	err = os.Mkdir(releaseDir, 0700)
	if err != nil {
		err = fmt.Errorf("failed to create release staging directory: %w", err)
		return
	}
	return
}

func prepareReleaseFiles(ctx *context) (releaseDir string, err error) {
	releaseDir, err = createReleaseStagingDir()
	if err != nil {
		return
	}

	searchPrefix := filepath.Join(ctx.repositoryRoot, ctx.cfg.ProgramLongPrefix+"*")
	compiledFiles, err := filepath.Glob(searchPrefix)
	if err != nil {
		err = fmt.Errorf("failed to find all compiled binaries: %w", err)
		return
	}

	for _, compiledFile := range compiledFiles {
		sourceFileName := filepath.Base(compiledFile)
		newPath := filepath.Join(releaseDir, sourceFileName)

		err = os.Rename(compiledFile, newPath)
		if err != nil {
			err = fmt.Errorf("failed to move compiled file to release staging: %w", err)
			return
		}
	}
	return
}

func prepareReleaseChangelog(ctx *context, releaseDir string) (err error) {
	releaseChangeLogFile := filepath.Join(releaseDir, "release-notes.md")

	printInfo(0, "Retrieving all git commit messages since last release...")

	// Read commit hash where last release was generated from
	releaseTrackerFile := filepath.Join(ctx.repositoryRoot, releaseCommitTracker)
	out, err := os.ReadFile(releaseTrackerFile)
	if err != nil {
		err = fmt.Errorf("failed to get commit of last release: %w", err)
		return
	}
	if len(out) == 0 {
		err = fmt.Errorf("no last release commit, aborting")
		return
	}
	lastReleaseCommitHash := string(bytes.Trim(out, "\n"))

	// Collect commit messages up until the last release commit (not including the release commit messages)
	cmd := exec.Command("git", "log", "--format=%B", lastReleaseCommitHash+"~0..HEAD")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("git log: %w: %s", err, string(out))
		return
	}
	commitMsgsSinceLastRelease := string(out)
	if commitMsgsSinceLastRelease == "" {
		// Return early if HEAD is where last release was generated (no messages to format)
		err = fmt.Errorf("no commits since last release")
		return
	}

	commitMsgs := strings.Split(commitMsgsSinceLastRelease, "\n")

	// Separate messages into categories based on prefix
	var added, changed, removed, fixed []string
	for _, commitMsg := range commitMsgs {
		if commitMsg == "" {
			// There will be empty lines
			continue
		}

		simpliedMsg := strings.ToLower(commitMsg)
		simpliedMsg = strings.Join(strings.Fields(simpliedMsg), " ")
		simpliedMsg = strings.TrimSpace(simpliedMsg)

		if strings.HasPrefix(simpliedMsg, "bump ") {
			continue
		}

		var targetCategory *[]string
		var removePrefix string
		switch {
		case strings.HasPrefix(simpliedMsg, "added "):
			targetCategory = &added
			removePrefix = "added "
		case strings.HasPrefix(simpliedMsg, "changed "):
			targetCategory = &changed
			removePrefix = "changed "
		case strings.HasPrefix(simpliedMsg, "removed "):
			targetCategory = &removed
			removePrefix = "removed "
		case strings.HasPrefix(simpliedMsg, "fixed "):
			commitMsg = strings.Replace(commitMsg, "bug where ", "", 1)
			targetCategory = &fixed
			removePrefix = "fixed "
		default:
			printWarn(4, "UNSUPPORTED LINE PREFIX: '%s'", commitMsg)
			continue
		}

		byteMsg := []byte(commitMsg)
		byteMsg = bytes.TrimPrefix(byteMsg, []byte(removePrefix))
		if len(byteMsg) == 0 {
			continue
		}
		firstChar := bytes.ToUpper([]byte{byteMsg[0]})
		byteMsg[0] = firstChar[0]
		formattedMsg := " - " + string(byteMsg)

		*targetCategory = append(*targetCategory, formattedMsg)
	}

	// Release Notes Section headers
	const addedHeader string = "### :white_check_mark: Added"
	const changedHeader string = "### :arrows_counterclockwise: Changed"
	const removedHeader string = "### :x: Removed"
	const fixedHeader string = "### :hammer: Fixed"
	const trailerHeader string = "### :information_source: Instructions"
	const trailerComment string = " - Please refer to the README.md file for instructions"

	var changeLog strings.Builder
	if len(added) > 0 {
		changeLog.WriteString(addedHeader)
		changeLog.WriteString("\n")
		changeLog.WriteString(strings.Join(added, "\n"))
		changeLog.WriteString("\n\n")
	}
	if len(changed) > 0 {
		changeLog.WriteString(changedHeader)
		changeLog.WriteString("\n")
		changeLog.WriteString(strings.Join(changed, "\n"))
		changeLog.WriteString("\n\n")
	}
	if len(removed) > 0 {
		changeLog.WriteString(removedHeader)
		changeLog.WriteString("\n")
		changeLog.WriteString(strings.Join(removed, "\n"))
		changeLog.WriteString("\n\n")
	}
	if len(fixed) > 0 {
		changeLog.WriteString(fixedHeader)
		changeLog.WriteString("\n")
		changeLog.WriteString(strings.Join(fixed, "\n"))
		changeLog.WriteString("\n\n")
	}

	changeLog.WriteString(trailerHeader)
	changeLog.WriteString("\n")
	changeLog.WriteString(trailerComment)
	changeLog.WriteString("\n")

	err = os.WriteFile(releaseChangeLogFile, []byte(changeLog.String()), 0600)
	if err != nil {
		err = fmt.Errorf("failed to write change log file: %w", err)
		return
	}

	// Save commit that this release was made for to track file
	cmd = exec.Command("git", "show", "HEAD", "--pretty=format:%H", "--no-patch")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("git show: %w: %s", err, string(out))
		return
	}
	currentReleaseCommitHash := bytes.Trim(out, "\n")

	err = os.WriteFile(releaseTrackerFile, currentReleaseCommitHash, 0600)
	if err != nil {
		err = fmt.Errorf("failed to save commit hash for this release: %w", err)
		return
	}

	fmt.Printf(`=====================================================================
RELEASE MESSAGE in %s - CHECK BEFORE PUBLISHING:
=====================================================================
%s
=====================================================================
RELEASE ATTACHMENTS in %s
=====================================================================
`, releaseChangeLogFile, changeLog.String(), releaseDir)

	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		err = fmt.Errorf("failed to read release staging directory: %w", err)
		return
	}

	for _, dirEntry := range entries {
		if dirEntry.IsDir() {
			continue
		}
		if dirEntry.Name() == filepath.Base(releaseChangeLogFile) {
			continue
		}
		fmt.Printf("%s\n", dirEntry.Name())
	}
	fmt.Println()
	printSuccess(0, "Done")
	return
}
