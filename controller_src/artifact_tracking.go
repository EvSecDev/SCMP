// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type GitArtifactTracker struct {
	PointerToArtifact   map[string]string
	PointerMapMutex     sync.Mutex
	PointerMetadata     map[string]string
	PointerMetaMapMutex sync.Mutex
	ArtifactHash        map[string]string
	ArtifactHashMutex   sync.Mutex
	AllErrors           []error
	ErrMutex            sync.Mutex
}

func (tracker *GitArtifactTracker) logError(err error) {
	tracker.ErrMutex.Lock()
	defer tracker.ErrMutex.Unlock()
	tracker.AllErrors = append(tracker.AllErrors, err)
}

func gitArtifactTracking() {
	// Get list of all files in repo ending in .remote-artifact
	artifactPointerFileNames, err := retrieveArtifactPointerFileNames()
	logError("Failed to retrieve list of remote artifact files", err, false)

	// Store artifact information and mapping between pointer and artifact file
	tracker := &GitArtifactTracker{
		PointerToArtifact: make(map[string]string),
		PointerMetadata:   make(map[string]string),
		ArtifactHash:      make(map[string]string),
	}

	// Concurrency sync
	var wg sync.WaitGroup

	// Map out each .remote-artifact file in repository and their info
	for _, artifactPointerFileName := range artifactPointerFileNames {
		wg.Add(1)
		go retrieveArtifactFileNames(&wg, artifactPointerFileName, tracker)
	}
	wg.Wait()

	// Check for errors
	if len(tracker.AllErrors) > 0 {
		printMessage(VerbosityStandard, "Error(s) while reading artifact pointers:\n")
		for _, err := range tracker.AllErrors {
			printMessage(VerbosityStandard, "  %v\n", err)
		}
		logError("Unable to continue", fmt.Errorf("too many errors"), false)
	}

	// Modify hash map so it only contains hashes of changed artifact files
	for artifactFileName, oldArtifactFileHash := range tracker.ArtifactHash {
		wg.Add(1)
		go hashArtifactFile(&wg, artifactFileName, oldArtifactFileHash, tracker)
	}
	wg.Wait()

	// Check for errors
	if len(tracker.AllErrors) > 0 {
		printMessage(VerbosityStandard, "Error(s) while hashing artifact files:\n")
		for _, err := range tracker.AllErrors {
			printMessage(VerbosityStandard, "  %v\n", err)
		}
		logError("Unable to continue", fmt.Errorf("too many errors"), false)
	}

	// Save any new artifact hashes into the artifact pointer file contents
	for artifactPointerFileName, artifactFileName := range tracker.PointerToArtifact {
		wg.Add(1)
		go writeUpdatedArtifactHash(&wg, artifactPointerFileName, artifactFileName, tracker)
	}
	wg.Wait()

	// Check for errors
	if len(tracker.AllErrors) > 0 {
		printMessage(VerbosityStandard, "Error(s) while writing new artifact hashes:\n")
		for _, err := range tracker.AllErrors {
			printMessage(VerbosityStandard, "  %v\n", err)
		}
		logError("Unable to continue", fmt.Errorf("too many errors"), false)
	}
}

// Walks entire git repository and creates array of any file ending in .remote-artifact
func retrieveArtifactPointerFileNames() (artifactPointerFileNames []string, err error) {
	// Walk through the repository to find all remote files
	err = filepath.Walk(config.RepositoryPath, func(path string, info os.FileInfo, err error) error {
		// Bail on any errors accessing directory
		if err != nil {
			return err
		}

		// Check if it's a file and has the .remote-artifact extension
		if !info.IsDir() && strings.HasSuffix(info.Name(), artifactPointerFileExtension) {
			artifactPointerFileNames = append(artifactPointerFileNames, path)
		}
		return nil
	})

	return
}

// Retrieve artifact file names from pointer
func retrieveArtifactFileNames(wg *sync.WaitGroup, artifactPointerFileName string, tracker *GitArtifactTracker) {
	// Signal routine is done after return
	defer wg.Done()

	// Retrieve tracked git file contents
	artifactPointerFileBytes, err := os.ReadFile(artifactPointerFileName)
	if err != nil {
		tracker.logError(err)
		return
	}

	// Grab metadata out of contents
	metadata, artifactPointerFileContent, err := extractMetadata(string(artifactPointerFileBytes))
	if err != nil {
		tracker.logError(fmt.Errorf("failed metadata extraction file %s: %v", artifactPointerFileName, err))
		return
	}

	// Get old hash from pointer file
	oldArtifactFileHash := SHA256RegEx.FindString(artifactPointerFileContent)

	// Safely write pointer file name to map
	tracker.PointerMetaMapMutex.Lock()
	tracker.PointerMetadata[artifactPointerFileName] = metadata
	tracker.PointerMetaMapMutex.Unlock()

	// Parse JSON into a generic map
	var jsonMetadata MetaHeader
	err = json.Unmarshal([]byte(metadata), &jsonMetadata)
	if err != nil {
		tracker.logError(fmt.Errorf("failed metadata parsing file %s: %v", artifactPointerFileName, err))
		return
	}

	// Ensure header has required location object
	if jsonMetadata.ExternalContentLocation == "" {
		tracker.logError(fmt.Errorf("'%s': JSON header is missing 'ExternalContentLocation' field", artifactPointerFileName))
		return
	}

	// Only allow file URIs for now
	if !strings.HasPrefix(jsonMetadata.ExternalContentLocation, fileURIPrefix) {
		tracker.logError(fmt.Errorf("'%s': must use '%s' before file paths in 'ExternalContentLocationput' field", artifactPointerFileName, fileURIPrefix))
		return
	}

	// Not adhering to actual URI standards -- I just want file paths
	artifactFileName := strings.TrimPrefix(jsonMetadata.ExternalContentLocation, fileURIPrefix)

	// Check for ~/ and expand if required
	artifactFileName = expandHomeDirectory(artifactFileName)

	// Save mapping of pointer file name to artifact file name
	tracker.PointerMapMutex.Lock()
	tracker.PointerToArtifact[artifactPointerFileName] = artifactFileName
	tracker.PointerMapMutex.Unlock()

	// Save old artifact hash into has map if not already present
	// Avoids unnecessary writes when many pointer files point to the same artifact file
	tracker.ArtifactHashMutex.Lock()
	_, artifactAlreadyInHashMap := tracker.ArtifactHash[artifactFileName]
	if !artifactAlreadyInHashMap {
		tracker.ArtifactHash[artifactFileName] = oldArtifactFileHash
	}
	tracker.ArtifactHashMutex.Unlock()
}

// Hash artifact file and update hash map
func hashArtifactFile(wg *sync.WaitGroup, artifactFileName string, oldArtifactFileHash string, tracker *GitArtifactTracker) {
	// Signal routine is done after return
	defer wg.Done()

	// Retrieve the hash of the current remote artifact file
	currrentArtifactFileHash, err := SHA256SumStream(artifactFileName)
	if err != nil {
		tracker.logError(fmt.Errorf("failed hasing artifact file %s: %v", artifactFileName, err))
		return
	}

	// Remove entry from hash map if artifact file is unchanged
	if currrentArtifactFileHash == oldArtifactFileHash {
		// Remove from hash map - only hashes that need updating should remain in the map
		tracker.ArtifactHashMutex.Lock()
		delete(tracker.ArtifactHash, artifactFileName)
		tracker.ArtifactHashMutex.Unlock()

		// Next artifact file
		return
	}

	// Overwrite old hash in hash map for this artifact
	tracker.ArtifactHashMutex.Lock()
	tracker.ArtifactHash[artifactFileName] = currrentArtifactFileHash
	tracker.ArtifactHashMutex.Unlock()
}

// Write new artifact hash to pointer file
func writeUpdatedArtifactHash(wg *sync.WaitGroup, artifactPointerFileName string, artifactFileName string, tracker *GitArtifactTracker) {
	// Signal routine is done after return
	defer wg.Done()

	// Skip pointer files where artifact hash has not changed - ArtifactHash should only have stale hashes left in it
	tracker.ArtifactHashMutex.Lock()
	_, StaleHashPresent := tracker.ArtifactHash[artifactFileName]
	tracker.ArtifactHashMutex.Unlock()
	if !StaleHashPresent {
		return
	}

	// Get original metadata from pointer
	tracker.PointerMetaMapMutex.Lock()
	metadata := tracker.PointerMetadata[artifactPointerFileName]
	tracker.PointerMetaMapMutex.Unlock()

	// Get New Artifact Hash
	tracker.ArtifactHashMutex.Lock()
	newArtifactHash := tracker.ArtifactHash[artifactFileName]
	tracker.ArtifactHashMutex.Unlock()

	// Combine existing metadata header with new artifact hash
	var builder strings.Builder
	builder.WriteString(Delimiter)
	builder.WriteString(metadata)
	builder.WriteString(Delimiter)
	builder.WriteString("\n")
	builder.WriteString(newArtifactHash)
	builder.WriteString("\n")

	// Write new artifact hash into pointer file
	err := os.WriteFile(artifactPointerFileName, []byte(builder.String()), 0600)
	if err != nil {
		tracker.logError(err)
		return
	}
}
