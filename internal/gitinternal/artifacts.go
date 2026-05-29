package gitinternal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/filesystem"
	"scmp/core/filesystem/content"
	"scmp/core/filesystem/metadata"
	"scmp/internal/crypto"
	"scmp/internal/fsops"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
	"sync"
)

type GitArtifactTracker struct {
	pointerToArtifact       map[str.LocalRepoPath]string
	pointerMapMutex         sync.Mutex
	pointerMetadata         map[str.LocalRepoPath]filesystem.MetaHeader
	pointerMetaMapMutex     sync.Mutex
	pointerCurrentHash      map[str.LocalRepoPath]string
	pointerCurrentHashMutex sync.Mutex
	artifactHash            map[string]string
	artifactHashMutex       sync.RWMutex
	allErrors               []error
	errMutex                sync.Mutex
}

func (tracker *GitArtifactTracker) logError(err error) {
	tracker.errMutex.Lock()
	defer tracker.errMutex.Unlock()
	tracker.allErrors = append(tracker.allErrors, err)
}

func checkForArtifactErrors(ctx context.Context, allErrors *[]error) (err error) {
	// Return early if no errors
	if len(*allErrors) == 0 {
		return
	}

	logctx.LogStdErr(ctx, "Error(s) while processing artifact files:\n")
	for _, err := range *allErrors {
		logctx.LogStdErr(ctx, "  %v\n", err)
	}
	err = fmt.Errorf("too many errors")
	return
}

// Walks entire git repository and creates array of any file ending in .remote-artifact
func retrieveArtifactPointerFileNames(repoPath string) (artifactPointerFileNames []str.LocalRepoPath, err error) {
	// Guard against no repository path
	if repoPath == "" {
		err = fmt.Errorf("could not identify git repository path, unable to track artifact files")
		return
	}

	// Walk through the repository to find all remote files
	err = filepath.Walk(string(repoPath), func(path string, info os.FileInfo, err error) error {
		// Bail on any errors accessing directory
		if err != nil {
			err = fmt.Errorf("failure encountered processing '%s': %w", path, err)
			return err
		}

		// Check if it's a file and has the .remote-artifact extension
		if !info.IsDir() && strings.HasSuffix(info.Name(), string(filesystem.ArtifactPointerFileExt)) {
			artifactPointerFileNames = append(artifactPointerFileNames, str.LocalRepoPath(path))
		}
		return nil
	})

	return
}

func ArtifactTracking(ctx context.Context, repoPath string) (err error) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSArtifacts)

	// Get list of all files in repo ending in .remote-artifact
	artifactPointerFileNames, err := retrieveArtifactPointerFileNames(repoPath)
	if err != nil {
		err = fmt.Errorf("error retrieving list of remote artifact files: %w", err)
		return
	}

	// Store artifact information and mapping between pointer and artifact file
	tracker := &GitArtifactTracker{
		pointerToArtifact:  make(map[str.LocalRepoPath]string),
		pointerMetadata:    make(map[str.LocalRepoPath]filesystem.MetaHeader),
		pointerCurrentHash: make(map[str.LocalRepoPath]string),
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
	err = checkForArtifactErrors(ctx, &tracker.allErrors)
	if err != nil {
		return
	}

	// Copy out keys for iteration
	var existingArtifactFiles []string
	for artifactFileName := range tracker.artifactHash {
		existingArtifactFiles = append(existingArtifactFiles, artifactFileName)
	}

	// Modify hash map so it only contains hashes of changed artifact files
	for _, artifactFileName := range existingArtifactFiles {
		wg.Add(1)
		go hashArtifactFile(ctx, &wg, semaphore, artifactFileName, tracker)
	}
	wg.Wait()
	err = checkForArtifactErrors(ctx, &tracker.allErrors)
	if err != nil {
		return
	}

	// Save any new artifact hashes into the artifact pointer file contents
	for artifactPointerFileName, artifactFileName := range tracker.pointerToArtifact {
		wg.Add(1)
		go writeUpdatedArtifactHash(ctx, &wg, semaphore, artifactPointerFileName, artifactFileName, tracker)
	}
	wg.Wait()
	err = checkForArtifactErrors(ctx, &tracker.allErrors)
	if err != nil {
		return
	}
	return
}

// ###################################
//  Go routines
// ###################################

// Retrieve artifact file names from pointer
func retrieveArtifactFileNames(wg *sync.WaitGroup, semaphore chan struct{}, artifactPointerFileName str.LocalRepoPath, tracker *GitArtifactTracker) {
	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	// Signal routine is done after return
	defer wg.Done()

	// Retrieve tracked git file contents
	artifactPointerFileBytes, err := os.ReadFile(string(artifactPointerFileName))
	if err != nil {
		tracker.logError(err)
		return
	}

	// Grab metadata out of contents
	jsonMetadata, artifactPointerFileContent, err := metadata.Extract(string(artifactPointerFileBytes))
	if err != nil {
		tracker.logError(fmt.Errorf("failed metadata extraction file %s: %w", artifactPointerFileName, err))
		return
	}

	// Get old hash from pointer file
	validHash, oldArtifactFileHash := parsing.HasHex64Prefix(string(artifactPointerFileContent))
	if !validHash && oldArtifactFileHash != "" {
		tracker.logError(fmt.Errorf("invalid hash retrieved from file %s: %w", artifactPointerFileName, err))
		return
	}

	// Safely write pointer file name to map
	tracker.pointerMetaMapMutex.Lock()
	tracker.pointerMetadata[artifactPointerFileName] = jsonMetadata
	tracker.pointerMetaMapMutex.Unlock()

	// Ensure header has required location object
	if jsonMetadata.ExternalContentLocation == "" {
		tracker.logError(fmt.Errorf("'%s': JSON header is missing 'ExternalContentLocation' field", artifactPointerFileName))
		return
	}

	// Only allow file URIs for now
	if !strings.HasPrefix(jsonMetadata.ExternalContentLocation, global.FileURIPrefix) {
		tracker.logError(fmt.Errorf("'%s': must use '%s' before file paths in 'ExternalContentLocationput' field", artifactPointerFileName, global.FileURIPrefix))
		return
	}

	// Not adhering to actual URI standards -- I just want file paths
	artifactFileName := strings.TrimPrefix(jsonMetadata.ExternalContentLocation, global.FileURIPrefix)

	// Check for ~/ and expand if required
	artifactFileName, err = fsops.ExpandHomeDirectory(artifactFileName)
	if err != nil {
		tracker.logError(fmt.Errorf("'%s': Unable to identify home directory for file '%s'", artifactPointerFileName, artifactFileName))
		return
	}

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
func hashArtifactFile(ctx context.Context, wg *sync.WaitGroup, semaphore chan struct{}, artifactFileName string, tracker *GitArtifactTracker) {
	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	// Signal routine is done after return
	defer wg.Done()

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Hashing artifact %s\n", artifactFileName)

	// Retrieve the hash of the current remote artifact file
	currentArtifactFileHash, err := crypto.SHA256SumStream(artifactFileName)
	if err != nil {
		tracker.logError(fmt.Errorf("failed hashing artifact file %s: %w", artifactFileName, err))
		return
	}

	// Add pointers hash to map by pointer name
	tracker.artifactHashMutex.Lock()
	tracker.artifactHash[artifactFileName] = currentArtifactFileHash
	tracker.artifactHashMutex.Unlock()
}

// Write new artifact hash to pointer file
func writeUpdatedArtifactHash(ctx context.Context, wg *sync.WaitGroup, semaphore chan struct{}, artifactPointerFileName str.LocalRepoPath, artifactFileName string, tracker *GitArtifactTracker) {
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
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Artifact pointer does not need it's hash updated (pointer: %s)\n", artifactPointerFileName)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Artifact pointer does need it's hash updated (pointer: %s)\n", artifactPointerFileName)

	// Get original metadata from pointer
	tracker.pointerMetaMapMutex.Lock()
	metadata := tracker.pointerMetadata[artifactPointerFileName]
	tracker.pointerMetaMapMutex.Unlock()

	// Get New Artifact Hash
	tracker.artifactHashMutex.Lock()
	newArtifactHash := tracker.artifactHash[artifactFileName]
	tracker.artifactHashMutex.Unlock()

	// Write new artifact hash into pointer file
	newArtifactHashBytes := []byte(newArtifactHash)
	err := content.WriteRepoFile(ctx, artifactPointerFileName, metadata, &newArtifactHashBytes)
	if err != nil {
		tracker.logError(err)
		return
	}
}
