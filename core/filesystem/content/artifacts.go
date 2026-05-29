package content

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/filesystem"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

func HandleArtifactFiles(ctx context.Context, localFilePath *str.LocalRepoPath, fileContents *[]byte, optCache map[string]int) (externalContentLocation string, err error) {
	fileIsPlainText := parsing.IsText(fileContents)

	// Return early if file is not an artifact
	if fileIsPlainText {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  File is plain text, not running artifact handling logic\n")
		return
	}

	// Repetitive artifact dirs - find most reused to suggest to user
	var mostReusedDir string
	var highestNum int
	for artifactDir, dirRepeatCnt := range optCache {
		if dirRepeatCnt < 2 {
			continue
		}

		if dirRepeatCnt > highestNum {
			highestNum = dirRepeatCnt
		}

		mostReusedDir = artifactDir
	}

	// Make file depending on if plain text or binary
	var userResponse string
	logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  File is not plain text, it should probably be stored outside of git\n")
	fmt.Print("  Specify a directory path where the actual file should be stored or enter 'none' to store file directly in repository\n")
	if mostReusedDir != "" {
		fmt.Printf("Default (press enter): '%v'\n", mostReusedDir)
	}

	fmt.Print("Path to External Directory: ")
	_, err = fmt.Scanln(&userResponse)
	if err != nil {
		err = fmt.Errorf("failed to get user response: %w", err)
		return
	}

	if strings.ToLower(userResponse) == "none" || (userResponse == "" && mostReusedDir == "") {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Did not receive an external content location for artifact, ARTIFACT CONTENTS WILL BE STORED IN REPOSITORY\n")
		return
	}

	if userResponse == "" {
		userResponse = mostReusedDir
	}

	// Ensure artifact fileContents are not written into repository
	defer func() {
		*fileContents = nil
	}()

	artifcateFileDirectory := str.LocalRepoPath(userResponse)
	remoteFileName := str.FilePathBase(*localFilePath)

	artifactFilePath := str.FilePathJoin(artifcateFileDirectory, remoteFileName)

	// Clean up user supplied path
	artifactFilePath, err = str.FilePathAbs(artifactFilePath)
	if err != nil {
		return
	}

	optCache[fmt.Sprintf("%v", artifactFilePath)]++

	// Store real file path in git-tracked file (set URI prefix)
	externalContentLocation = global.FileURIPrefix + string(artifactFilePath)

	err = os.MkdirAll(filepath.Dir(string(artifactFilePath)), 0750)
	if err != nil {
		return
	}

	artifactFile, err := os.OpenFile(string(artifactFilePath), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer func() {
		_ = artifactFile.Close()
	}()

	_, err = artifactFile.Write(*fileContents)
	if err != nil {
		return
	}

	// Add extension to mark file as external
	*localFilePath += filesystem.ArtifactPointerFileExt

	return
}
