// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (tracker *GitArtifactTracker) logError(err error) {
	tracker.errMutex.Lock()
	defer tracker.errMutex.Unlock()
	tracker.allErrors = append(tracker.allErrors, err)
}

func checkForArtifactErrors(allErrors *[]error) {
	// Return early if no errors
	if len(*allErrors) == 0 {
		return
	}

	printMessage(verbosityStandard, "Error(s) while processing artifact files:\n")
	for _, err := range *allErrors {
		printMessage(verbosityStandard, "  %v\n", err)
	}
	logError("Unable to continue", fmt.Errorf("too many errors"), false)

}

// Walks entire git repository and creates array of any file ending in .remote-artifact
func retrieveArtifactPointerFileNames() (artifactPointerFileNames []string, err error) {
	// Guard against no repository path
	if config.repositoryPath == "" {
		err = fmt.Errorf("could not identify git repository path, unable to track artifact files")
		return
	}

	// Walk through the repository to find all remote files
	err = filepath.Walk(config.repositoryPath, func(path string, info os.FileInfo, err error) error {
		// Bail on any errors accessing directory
		if err != nil {
			err = fmt.Errorf("failure encountered processing '%s': %v", path, err)
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
