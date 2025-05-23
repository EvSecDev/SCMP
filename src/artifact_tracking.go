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
	pointerToArtifact       map[string]string
	pointerMapMutex         sync.Mutex
	pointerMetadata         map[string]string
	pointerMetaMapMutex     sync.Mutex
	pointerCurrentHash      map[string]string
	pointerCurrentHashMutex sync.Mutex
	artifactHash            map[string]string
	artifactHashMutex       sync.Mutex
	allErrors               []error
	errMutex                sync.Mutex
}

func gitArtifactTracking() {
	// Get list of all files in repo ending in .remote-artifact
	artifactPointerFileNames, err := retrieveArtifactPointerFileNames()
	logError("Failed to retrieve list of remote artifact files", err, false)

	// Store artifact information and mapping between pointer and artifact file
	tracker := &GitArtifactTracker{
		pointerToArtifact:  make(map[string]string),
		pointerMetadata:    make(map[string]string),
		pointerCurrentHash: make(map[string]string),
		artifactHash:       make(map[string]string),
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
	copyTrackerArtifactHash := tracker.artifactHash
	for artifactFileName := range copyTrackerArtifactHash {
		wg.Add(1)
		go hashArtifactFile(&wg, semaphore, artifactFileName, tracker)
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

	// Add pointer file content current hash to map of pointer names
	tracker.pointerCurrentHashMutex.Lock()
	tracker.pointerCurrentHash[artifactPointerFileName] = oldArtifactFileHash
	tracker.pointerCurrentHashMutex.Unlock()

	// Save old artifact hash into hash map if not already present
	tracker.artifactHashMutex.Lock()
	_, artifactAlreadyInHashMap := tracker.artifactHash[artifactFileName]
	if !artifactAlreadyInHashMap {
		tracker.artifactHash[artifactFileName] = oldArtifactFileHash
	}
	tracker.artifactHashMutex.Unlock()
}

// Hash artifact file and update hash map
func hashArtifactFile(wg *sync.WaitGroup, semaphore chan struct{}, artifactFileName string, tracker *GitArtifactTracker) {
	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	// Signal routine is done after return
	defer wg.Done()

	printMessage(verbosityData, "Hashing artifact %s\n", artifactFileName)

	// Retrieve the hash of the current remote artifact file
	currentArtifactFileHash, err := SHA256SumStream(artifactFileName)
	if err != nil {
		tracker.logError(fmt.Errorf("failed hashing artifact file %s: %v", artifactFileName, err))
		return
	}

	// Add pointers hash to map by pointer name
	tracker.artifactHashMutex.Lock()
	tracker.artifactHash[artifactFileName] = currentArtifactFileHash
	tracker.artifactHashMutex.Unlock()
}

// Write new artifact hash to pointer file
func writeUpdatedArtifactHash(wg *sync.WaitGroup, semaphore chan struct{}, artifactPointerFileName string, artifactFileName string, tracker *GitArtifactTracker) {
	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	// Signal routine is done after return
	defer wg.Done()

	// Skip pointer files where hash in file matches hash of artifact
	tracker.artifactHashMutex.Lock()
	currentArtifactHash := tracker.artifactHash[artifactFileName]
	tracker.artifactHashMutex.Unlock()

	tracker.pointerCurrentHashMutex.Lock()
	currentArtifactPointerHash := tracker.pointerCurrentHash[artifactPointerFileName]
	tracker.pointerCurrentHashMutex.Unlock()

	if currentArtifactHash == currentArtifactPointerHash {
		printMessage(verbosityProgress, "Artifact pointer does not need it's hash updated (pointer: %s)\n", artifactPointerFileName)
		return
	}

	printMessage(verbosityProgress, "Artifact pointer does need it's hash updated (pointer: %s)\n", artifactPointerFileName)

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
