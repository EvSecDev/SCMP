// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Function to extract metadata JSON from file contents
func extractMetadata(fileContents string) (metadata MetaHeader, contentSection []byte, err error) {
	printMessage(verbosityData, "    Extracting file metadata\n")

	// Do not allow carriage returns
	fileContents = strings.ReplaceAll(fileContents, "\r", "")

	// Handle comments around metadata header
	fileContents = strings.Replace(fileContents, "/*"+metaDelimiter, metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, metaDelimiter+"*/", metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, "<!--"+metaDelimiter, metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, metaDelimiter+"-->", metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, "#"+metaDelimiter, metaDelimiter, 2)
	fileContents = strings.Replace(fileContents, ";"+metaDelimiter, metaDelimiter, 2)
	fileContents = strings.Replace(fileContents, "//"+metaDelimiter, metaDelimiter, 2)

	// Find the start and end of the metadata section
	startIndex := strings.Index(fileContents, metaDelimiter)
	if startIndex == -1 {
		err = fmt.Errorf("json start delimiter missing")
		return
	}
	startIndex += len(metaDelimiter)

	endIndex := strings.Index(fileContents[startIndex:], metaDelimiter)
	if endIndex == -1 {
		testEndIndex := strings.Index(fileContents[startIndex:], metaDelimiter)
		if testEndIndex == -1 {
			err = fmt.Errorf("json end delimiter missing")
			return
		}
		err = fmt.Errorf("json end delimiter missing")
		return
	}
	endIndex += startIndex

	// Extract the metadata section
	metadataSection := fileContents[startIndex:endIndex]

	// Handle commented out metadata lines
	metadataSection = strings.ReplaceAll(metadataSection, "\n#", "\n")
	metadataSection = strings.ReplaceAll(metadataSection, "\n//", "\n")
	metadataSection = strings.ReplaceAll(metadataSection, "\n;", "\n")

	printMessage(verbosityData, "    Parsing metadata header JSON\n")

	err = json.Unmarshal([]byte(metadataSection), &metadata)
	if err != nil {
		err = fmt.Errorf("invalid metadata header: %v", err)
		return
	}

	// Extract the content section
	remainingContent := fileContents[:startIndex-len(metaDelimiter)] + fileContents[endIndex+len(metaDelimiter):]
	remainingContent = strings.TrimPrefix(remainingContent, "\n")

	contentSection = []byte(remainingContent)

	return
}

// Writes file contents to repository file with added metadata header
// File content optional
func writeLocalRepoFile(localFilePath string, metadata MetaHeader, fileContent *[]byte) (err error) {
	printMessage(verbosityProgress, "Adding JSON metadata header to file '%s'\n", localFilePath)

	var startDelimiter, endDelimiter, metaPrefix string
	switch filepath.Ext(localFilePath) {
	case ".sh", ".zsh", ".bashrc", ".zshrc", ".yaml", ".yml", ".py":
		metaPrefix = "#"
		startDelimiter = metaDelimiter
		endDelimiter = metaDelimiter
	case ".html", ".htm", ".xml":
		startDelimiter = "<!--" + metaDelimiter
		endDelimiter = metaDelimiter + "-->"
	case ".go", ".css", ".js", ".php":
		startDelimiter = "/*" + metaDelimiter
		endDelimiter = metaDelimiter + "*/"
	default:
		startDelimiter = metaDelimiter
		endDelimiter = metaDelimiter
	}

	metaHeaderBytes, err := json.MarshalIndent(metadata, metaPrefix, "  ")
	if err != nil {
		err = fmt.Errorf("error parsing metadata header: %v", err)
	}
	header := string(metaHeaderBytes)

	var fullFileContent strings.Builder
	fullFileContent.WriteString(startDelimiter)
	fullFileContent.WriteString("\n")
	fullFileContent.WriteString(metaPrefix) // JSON MarshalIndent does not add prefix to the first line
	fullFileContent.WriteString(header)
	fullFileContent.WriteString("\n")
	fullFileContent.WriteString(endDelimiter)
	fullFileContent.WriteString("\n")
	if fileContent != nil {
		fullFileContent.WriteString(string(*fileContent))
	}

	printMessage(verbosityProgress, "Writing file '%s' to repository\n", localFilePath)

	configParentDirs := filepath.Dir(localFilePath)
	err = os.MkdirAll(configParentDirs, 0700)
	if err != nil {
		err = fmt.Errorf("failed to create missing parent directories in local repository: %v", err)
		return
	}

	localFile, err := os.OpenFile(localFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		err = fmt.Errorf("failed to open/create directory metadata file: %v", err)
		return
	}
	defer localFile.Close()

	_, err = localFile.WriteString(fullFileContent.String())
	if err != nil {
		err = fmt.Errorf("failed to write file to local repository: %v", err)
		return
	}

	return
}

func writeSymbolicLinkToRepo(localLinkPath string, selectionMetadata RemoteFileInfo) (err error) {
	var linkMetadata MetaHeader
	linkMetadata.SymbolicLinkTarget = selectionMetadata.linkTarget
	linkMetadata.TargetFileOwnerGroup = "root:root"
	linkMetadata.TargetFilePermissions = 777

	printMessage(verbosityData, "  Symbolic Link '%s': Target: %s\n", localLinkPath, selectionMetadata.linkTarget)

	err = writeLocalRepoFile(localLinkPath, linkMetadata, nil)
	if err != nil {
		err = fmt.Errorf("failed to write directory metadata to local repository: %v", err)
		return
	}

	return
}

// Writes directory metadata of chosen dir to repo
func writeNewDirectoryMetadata(localDirPath string, selectionMetadata RemoteFileInfo) (err error) {
	var dirMetadata MetaHeader
	dirMetadata.TargetFileOwnerGroup = selectionMetadata.owner + ":" + selectionMetadata.group
	dirMetadata.TargetFilePermissions = selectionMetadata.permissions

	printMessage(verbosityData, "  Directory '%s': Metadata: %d %s\n", localDirPath, selectionMetadata.permissions, dirMetadata.TargetFileOwnerGroup)

	metadataFile := filepath.Join(localDirPath, directoryMetadataFileName)

	err = writeLocalRepoFile(metadataFile, dirMetadata, nil)
	if err != nil {
		err = fmt.Errorf("failed to write directory metadata to local repository: %v", err)
		return
	}

	return
}

func handleArtifactFiles(localFilePath *string, fileContents *[]byte, optCache *SeedRepoUserChoiceCache) (externalContentLocation string, err error) {
	fileIsPlainText := isText(fileContents)

	// Return early if file is not an artifact
	if fileIsPlainText {
		printMessage(verbosityProgress, "  File is plain text, not running artifact handling logic\n")
		return
	}

	// Repetitive artifact dirs - find most reused to suggest to user
	var mostReusedDir string
	var highestNum int
	for artifactDir, dirRepeatCnt := range optCache.artifactExtDir {
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
	printMessage(verbosityStandard, "  File is not plain text, it should probably be stored outside of git\n")
	fmt.Print("  Specify a directory path where the actual file should be stored or enter 'none' to store file directly in repository\n")
	if mostReusedDir != "" {
		fmt.Printf("Default (press enter): '%v'\n", mostReusedDir)
	}

	fmt.Print("Path to External Directory: ")
	fmt.Scanln(&userResponse)

	if strings.ToLower(userResponse) == "none" || (userResponse == "" && mostReusedDir == "") {
		printMessage(verbosityProgress, "  Did not receive an external content location for artifact, ARTIFACT CONTENTS WILL BE STORED IN REPOSITORY\n")
		return
	}

	if userResponse == "" {
		userResponse = mostReusedDir
	}

	// Ensure artifact fileContents are not written into repository
	defer func() {
		*fileContents = nil
	}()

	artifcateFileDirectory := userResponse
	remoteFileName := filepath.Base(*localFilePath)

	artifactFilePath := filepath.Join(artifcateFileDirectory, remoteFileName)

	// Clean up user supplied path
	artifactFilePath, err = filepath.Abs(artifactFilePath)
	if err != nil {
		return
	}

	optCache.artifactExtDir[fmt.Sprintf("%v", artifactFilePath)]++

	// Store real file path in git-tracked file (set URI prefix)
	externalContentLocation = fileURIPrefix + artifactFilePath

	err = os.MkdirAll(filepath.Dir(artifactFilePath), 0750)
	if err != nil {
		return
	}

	artifactFile, err := os.OpenFile(artifactFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer artifactFile.Close()

	_, err = artifactFile.Write(*fileContents)
	if err != nil {
		return
	}

	// Add extension to mark file as external
	*localFilePath += artifactPointerFileExtension

	return
}
