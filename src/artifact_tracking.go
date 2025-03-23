// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type GitArtifactTracker struct {
	pointerToArtifact   map[string]string
	pointerMapMutex     sync.Mutex
	pointerMetadata     map[string]string
	pointerMetaMapMutex sync.Mutex
	artifactHash        map[string]string
	artifactHashMutex   sync.Mutex
	allErrors           []error
	errMutex            sync.Mutex
}

func gitArtifactTracking() {
	// Get list of all files in repo ending in .remote-artifact
	artifactPointerFileNames, err := retrieveArtifactPointerFileNames()
	logError("Failed to retrieve list of remote artifact files", err, false)

	// Store artifact information and mapping between pointer and artifact file
	tracker := &GitArtifactTracker{
		pointerToArtifact: make(map[string]string),
		pointerMetadata:   make(map[string]string),
		artifactHash:      make(map[string]string),
	}

	// Concurrency
	const maxArtifactConcurrency int = 250                   // Limit concurrency to sane limit - number of max files in flight
	semaphore := make(chan struct{}, maxArtifactConcurrency) // Semaphore to limit concurrency
	var wg sync.WaitGroup                                    // Concurrency sync

	// Map out each .remote-artifact file in repository and their info
	for _, artifactPointerFileName := range artifactPointerFileNames {
		wg.Add(1)
		go retrieveArtifactFileNames(&wg, semaphore, artifactPointerFileName, tracker)
	}
	wg.Wait()
	checkForArtifactErrors(&tracker.allErrors)

	// Modify hash map so it only contains hashes of changed artifact files
	for artifactFileName, oldArtifactFileHash := range tracker.artifactHash {
		wg.Add(1)
		go hashArtifactFile(&wg, semaphore, artifactFileName, oldArtifactFileHash, tracker)
	}
	wg.Wait()
	checkForArtifactErrors(&tracker.allErrors)

	// Save any new artifact hashes into the artifact pointer file contents
	for artifactPointerFileName, artifactFileName := range tracker.pointerToArtifact {
		wg.Add(1)
		go writeUpdatedArtifactHash(&wg, semaphore, artifactPointerFileName, artifactFileName, tracker)
	}
	wg.Wait()
	checkForArtifactErrors(&tracker.allErrors)
}

// ###################################
//  Go routines
// ###################################

// Retrieve artifact file names from pointer
func retrieveArtifactFileNames(wg *sync.WaitGroup, semaphore chan struct{}, artifactPointerFileName string, tracker *GitArtifactTracker) {
	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

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
	oldArtifactFileHash := SHA256RegEx.FindString(string(artifactPointerFileContent))

	// Safely write pointer file name to map
	tracker.pointerMetaMapMutex.Lock()
	tracker.pointerMetadata[artifactPointerFileName] = metadata
	tracker.pointerMetaMapMutex.Unlock()

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
	tracker.pointerMapMutex.Lock()
	tracker.pointerToArtifact[artifactPointerFileName] = artifactFileName
	tracker.pointerMapMutex.Unlock()

	// Save old artifact hash into has map if not already present
	// Avoids unnecessary writes when many pointer files point to the same artifact file
	tracker.artifactHashMutex.Lock()
	_, artifactAlreadyInHashMap := tracker.artifactHash[artifactFileName]
	if !artifactAlreadyInHashMap {
		tracker.artifactHash[artifactFileName] = oldArtifactFileHash
	}
	tracker.artifactHashMutex.Unlock()
}

// Hash artifact file and update hash map
func hashArtifactFile(wg *sync.WaitGroup, semaphore chan struct{}, artifactFileName string, oldArtifactFileHash string, tracker *GitArtifactTracker) {
	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

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
		tracker.artifactHashMutex.Lock()
		delete(tracker.artifactHash, artifactFileName)
		tracker.artifactHashMutex.Unlock()

		// Next artifact file
		return
	}

	// Overwrite old hash in hash map for this artifact
	tracker.artifactHashMutex.Lock()
	tracker.artifactHash[artifactFileName] = currrentArtifactFileHash
	tracker.artifactHashMutex.Unlock()
}

// Write new artifact hash to pointer file
func writeUpdatedArtifactHash(wg *sync.WaitGroup, semaphore chan struct{}, artifactPointerFileName string, artifactFileName string, tracker *GitArtifactTracker) {
	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	// Signal routine is done after return
	defer wg.Done()

	// Skip pointer files where artifact hash has not changed - ArtifactHash should only have stale hashes left in it
	tracker.artifactHashMutex.Lock()
	_, StaleHashPresent := tracker.artifactHash[artifactFileName]
	tracker.artifactHashMutex.Unlock()
	if !StaleHashPresent {
		return
	}

	// Get original metadata from pointer
	tracker.pointerMetaMapMutex.Lock()
	metadata := tracker.pointerMetadata[artifactPointerFileName]
	tracker.pointerMetaMapMutex.Unlock()

	// Get New Artifact Hash
	tracker.artifactHashMutex.Lock()
	newArtifactHash := tracker.artifactHash[artifactFileName]
	tracker.artifactHashMutex.Unlock()

	// Combine existing metadata header with new artifact hash
	var builder strings.Builder
	builder.WriteString(metaDelimiter)
	builder.WriteString(metadata)
	builder.WriteString(metaDelimiter)
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
