// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ###################################
//   DEPLOY LAST FAILURE
// ###################################

// Reads fail tracker file and uses embedded commit, hosts, and files to redeploy
func failureDeployment(config Config, hostOverride string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while retrying failed deployment", fmt.Errorf("%v", fatalError), true)
		}
	}()

	// Show progress to user
	fmt.Printf("%s\n", progCLIHeader)
	fmt.Print("     Starting failure deployment\n")
	fmt.Print("Running local system checks... ")

	// Ensure local system is in a state that is able to deploy
	err := localSystemChecks()
	logError("Error in local system checks", err, true)

	// Regex to match commitid line from fail tracker
	failCommitRegEx := regexp.MustCompile(`commitid:([0-9a-fA-F]+)\n`)

	// Read in contents of fail tracker file
	failTrackerPath := filepath.Join(RepositoryPath, FailTrackerFile)
	lastFailTrackerBytes, err := os.ReadFile(failTrackerPath)
	logError("Failed to read last fail tracker file", err, false)

	// Convert tracker to string
	lastFailTracker := string(lastFailTrackerBytes)

	// Use regex to extract commit hash from line in fail tracker (should be the first line)
	commitRegexMatches := failCommitRegEx.FindStringSubmatch(lastFailTracker)

	// Save the retrieved ID to the string and the raw hash
	commitID := commitRegexMatches[1]
	commitHash := plumbing.NewHash(commitRegexMatches[1])

	// Remove commit line from the failtracker contents using the commit regex
	lastFailTracker = failCommitRegEx.ReplaceAllString(lastFailTracker, "")

	// Open the repository
	repo, err := git.PlainOpen(RepositoryPath)
	logError("Failed to open repository", err, true)

	// Get the commit
	commit, err := repo.CommitObject(commitHash)
	logError("Failed to get commit object", err, true)

	// Get the tree from the commit
	tree, err := commit.Tree()
	logError("Failed to get commit tree", err, true)

	// Show progress to user
	fmt.Print("Complete.\n")

	// Retrieve hosts and files from failtracker
	commitFiles, commitHostNames, err := getCommitFailures(lastFailTracker)
	logError("Failed to retrieve failed commit files", err, true)

	// Retrieve all repository files - for use in deduplicating host and universal files
	repoHostsandFiles, err := mapAllRepoFiles(tree)
	logError("Failed to retrieve all repository files", err, true)

	// Create maps of deployment files and hosts
	var preDeploymentHosts int
	hostsAndFilePaths, hostsAndEndpointInfo, targetEndpoints, allLocalFiles, err := getHostsAndFiles(commitFiles, commitHostNames, repoHostsandFiles, config.DeployerEndpoints, hostOverride, config.SSHClientDefault, &preDeploymentHosts)
	logError("Failed to get host and files", err, true)

	// Load the files for deployment
	var preDeployedConfigs int
	commitFileInfo, err := loadFiles(allLocalFiles, commitFiles, tree, &preDeployedConfigs)
	logError("Error loading files", err, true)

	// Show progress to user - using the metrics
	fmt.Printf("Beginning deployment of %d configuration(s) to %d host(s)\n", preDeployedConfigs, preDeploymentHosts)

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, config.SSHClient.MaximumConcurrency)

	// If user requested dry run - print collected information so far and gracefully abort deployment
	if dryRunRequested {
		printDeploymentInformation(targetEndpoints, hostsAndFilePaths, hostsAndEndpointInfo, commitFileInfo)
		fmt.Printf("================================================\n")
		return
	}

	// Start go routines for each remote host ssh
	var wg sync.WaitGroup
	for _, endpointName := range targetEndpoints {
		// Start go routine for specific host
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		go deployConfigs(&wg, semaphore, endpointName, hostsAndFilePaths[endpointName], hostsAndEndpointInfo[endpointName], commitFileInfo)
	}
	wg.Wait()

	// Save deployment errors to fail tracker
	if FailTracker != "" {
		err := recordDeploymentError(commitID)
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		return
	}

	// Remove fail tracker file after successful redeployment - removal errors don't matter.
	os.Remove(failTrackerPath)

	// Show progress to user
	fmt.Printf("\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	fmt.Printf("================================================\n")
}

// ###################################
//   DEPLOY ALL FILES BY COMMIT
// ###################################

// Deploys chosen files to chosen hosts for a given commit (even unchanged files are available for deployment)
func allDeployment(config Config, commitID string, hostOverride string, fileOverride string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while deploying all files", fmt.Errorf("%v", fatalError), true)
		}
	}()

	// Show progress to user
	fmt.Printf("%s\n", progCLIHeader)
	fmt.Print("     Starting all deployment\n")
	fmt.Print("Running local system checks... ")

	// Ensure local system is in a state that is able to deploy
	err := localSystemChecks()
	logError("Error in local system checks", err, true)

	// Open the repository
	repo, err := git.PlainOpen(RepositoryPath)
	logError("Failed to open repository", err, true)

	// If user did not supply a commitID, assume they want to use the HEAD commit
	if commitID == "" {
		// Get the pointer to the HEAD commit
		ref, err := repo.Head()
		logError("Failed to get HEAD reference", err, true)

		// Save HEAD commitID
		commitID = ref.Hash().String()
	}

	// Verify commit ID string content - only truly required when user specifies it - but verify anyways
	if !SHA1RegEx.MatchString(commitID) {
		logError("Error with supplied commit ID", fmt.Errorf("hash is not 40 characters and/or is not hexadecimal"), true)
	}

	// Set hash
	commitHash := plumbing.NewHash(commitID)

	// Get the commit
	commit, err := repo.CommitObject(commitHash)
	logError("Failed to get commit object", err, true)

	// Get the tree from the commit
	tree, err := commit.Tree()
	logError("Failed to get commit tree", err, true)

	// Show progress to user
	fmt.Print("Complete.\n")

	// Retrieve all repository files
	commitFiles, repoHostsandFiles, err := getRepoFiles(tree, fileOverride)
	logError("Failed to retrieve all repository files", err, true)

	// Create array of just host names for filtering endpoints
	var commitHostNames []string
	for host := range repoHostsandFiles {
		commitHostNames = append(commitHostNames, host)
	}

	// Create maps of deployment files and hosts
	var preDeploymentHosts int
	hostsAndFilePaths, hostsAndEndpointInfo, targetEndpoints, allLocalFiles, err := getHostsAndFiles(commitFiles, commitHostNames, repoHostsandFiles, config.DeployerEndpoints, hostOverride, config.SSHClientDefault, &preDeploymentHosts)
	logError("Failed to get host and files", err, true)

	// Load the files for deployment
	var preDeployedConfigs int
	commitFileInfo, err := loadFiles(allLocalFiles, commitFiles, tree, &preDeployedConfigs)
	logError("Error loading files", err, true)

	// Show progress to user - using the metrics
	fmt.Printf("Beginning deployment of %d configuration(s) to %d host(s)\n", preDeployedConfigs, preDeploymentHosts)

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, config.SSHClient.MaximumConcurrency)

	// If user requested dry run - print collected information so far and gracefully abort deployment
	if dryRunRequested {
		printDeploymentInformation(targetEndpoints, hostsAndFilePaths, hostsAndEndpointInfo, commitFileInfo)
		fmt.Printf("================================================\n")
		return
	}

	// Start go routines for each remote host ssh
	var wg sync.WaitGroup
	for _, endpointName := range targetEndpoints {
		// Start go routine for specific host
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		go deployConfigs(&wg, semaphore, endpointName, hostsAndFilePaths[endpointName], hostsAndEndpointInfo[endpointName], commitFileInfo)
	}
	wg.Wait()

	// Save deployment errors to fail tracker
	if FailTracker != "" {
		err := recordDeploymentError(commitID)
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		return
	}

	// Show progress to user
	fmt.Printf("\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	fmt.Print("================================================\n")
}

// ###################################
//      AUTOMATIC - USE HEAD COMMIT
// ###################################

// Main entry point for git post-commit hook
// Assumes desired commit is head commit
// Only deploys changed files between head commit and previous commit
func autoDeployment(config Config, hostOverride string, fileOverride string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while processing automatic deployment", fmt.Errorf("%v", fatalError), true)
		}
	}()

	// Show progress to user
	fmt.Printf("%s\n", progCLIHeader)
	fmt.Print("     Starting automatic deployment\n")
	fmt.Print("Running local system checks... ")

	// Ensure local system is in a state that is able to deploy
	err := localSystemChecks()
	logError("Error in local system checks", err, true)

	// Open the repository
	repo, err := git.PlainOpen(RepositoryPath)
	logError("Failed to open repository", err, true)

	// Get the pointer to the HEAD commit
	ref, err := repo.Head()
	logError("Failed to get HEAD reference", err, true)

	// Commit Hash string version for fail tracker output
	commitID := ref.Hash().String()

	// Get the commit
	commit, err := repo.CommitObject(ref.Hash())
	logError("Failed to get commit object", err, true)

	// Get the tree from the commit
	tree, err := commit.Tree()
	logError("Failed to get commit tree", err, true)

	// Show progress to user
	fmt.Print("Complete.\n")

	// Retrieve and validate committed files
	commitFiles, commitHostNames, err := getCommitFiles(commit, config.DeployerEndpoints, fileOverride)
	if err != nil {
		if err.Error() == "no valid files in commit" {
			// exit the program when no files - usually when committing files outside of host directories
			fmt.Print("No files available for deployment.\n")
			fmt.Print("================================================\n")
			return
		}
		// Rollback and exit for other errors
		logError("Failed to retrieve commit files", err, true)
	}

	// Retrieve all repository files
	repoHostsandFiles, err := mapAllRepoFiles(tree)
	logError("Failed to retrieve all repository files", err, true)

	// Create maps of deployment files and hosts
	var preDeploymentHosts int
	hostsAndFilePaths, hostsAndEndpointInfo, targetEndpoints, allLocalFiles, err := getHostsAndFiles(commitFiles, commitHostNames, repoHostsandFiles, config.DeployerEndpoints, hostOverride, config.SSHClientDefault, &preDeploymentHosts)
	logError("Failed to get host and files", err, true)

	// Load the files for deployment
	var preDeployedConfigs int
	commitFileInfo, err := loadFiles(allLocalFiles, commitFiles, tree, &preDeployedConfigs)
	logError("Error loading files", err, true)

	// Show progress to user - using the metrics
	fmt.Printf("Beginning deployment of %d configuration(s) to %d host(s)\n", preDeployedConfigs, preDeploymentHosts)

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, config.SSHClient.MaximumConcurrency)

	// If user requested dry run - print collected information so far and gracefully abort deployment
	if dryRunRequested {
		printDeploymentInformation(targetEndpoints, hostsAndFilePaths, hostsAndEndpointInfo, commitFileInfo)
		fmt.Printf("================================================\n")
		return
	}

	// Start go routines for each remote host ssh
	var wg sync.WaitGroup
	for _, endpointName := range targetEndpoints {
		// Start go routine for specific host
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		go deployConfigs(&wg, semaphore, endpointName, hostsAndFilePaths[endpointName], hostsAndEndpointInfo[endpointName], commitFileInfo)
	}
	wg.Wait()

	// Save deployment errors to fail tracker
	if FailTracker != "" {
		err := recordDeploymentError(commitID)
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		return
	}

	// Show progress to user
	fmt.Printf("\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	fmt.Print("================================================\n")
}

// ###################################
//  DEPLOY CHANGED FILES IN A COMMIT
// ###################################

// User chosen commit deployment (and possibly hosts and/or files)
// Only deploys changed files between chosen commit and previous commit
func manualDeployment(config Config, commitID string, hostOverride string, fileOverride string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while processing manual deployment", fmt.Errorf("%v", fatalError), true)
		}
	}()

	// Show progress to user
	fmt.Printf("%s\n", progCLIHeader)
	fmt.Printf("     Starting manual deployment")
	fmt.Printf("     Commit %s\n", commitID)
	fmt.Print("Running local system checks... ")

	// Verify commit ID string content from  user
	if !SHA1RegEx.MatchString(commitID) {
		logError("Error with supplied commit ID", fmt.Errorf("hash is not 40 characters and/or is not hexadecimal"), true)
	}

	// Ensure local system is in a state that is able to deploy
	err := localSystemChecks()
	logError("Error in local system checks", err, true)

	// Open the repository
	repo, err := git.PlainOpen(RepositoryPath)
	logError("Failed to open repository", err, true)

	// Get the commit
	commit, err := repo.CommitObject(plumbing.NewHash(commitID))
	logError("Failed to get commit object", err, true)

	// Get the tree from the commit
	tree, err := commit.Tree()
	logError("Failed to get commit tree", err, true)

	// Show progress to user
	fmt.Print("Complete.\n")

	// Retrieve and validate committed files
	commitFiles, commitHostNames, err := getCommitFiles(commit, config.DeployerEndpoints, fileOverride)
	if err != nil {
		if err.Error() == "no valid files in commit" {
			// exit the program when no files - usually when committing files outside of host directories
			fmt.Printf("No files available for deployment.\n")
			fmt.Printf("================================================\n")
			return
		}
		// Rollback and exit for other errors
		logError("Failed to retrieve commit files", err, true)
	}

	// Retrieve all repository files
	repoHostsandFiles, err := mapAllRepoFiles(tree)
	logError("Failed to retrieve all repository files", err, true)

	// Create maps of deployment files and hosts
	var preDeploymentHosts int
	hostsAndFilePaths, hostsAndEndpointInfo, targetEndpoints, allLocalFiles, err := getHostsAndFiles(commitFiles, commitHostNames, repoHostsandFiles, config.DeployerEndpoints, hostOverride, config.SSHClientDefault, &preDeploymentHosts)
	logError("Failed to get host and files", err, true)

	// Load the files for deployment
	var preDeployedConfigs int
	commitFileInfo, err := loadFiles(allLocalFiles, commitFiles, tree, &preDeployedConfigs)
	logError("Error loading files", err, true)

	// Show progress to user - using the metrics
	fmt.Printf("Beginning deployment of %d configuration(s) to %d host(s)\n", preDeployedConfigs, preDeploymentHosts)

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, config.SSHClient.MaximumConcurrency)

	// If user requested dry run - print collected information so far and gracefully abort deployment
	if dryRunRequested {
		printDeploymentInformation(targetEndpoints, hostsAndFilePaths, hostsAndEndpointInfo, commitFileInfo)
		fmt.Printf("================================================\n")
		return
	}

	// Start go routines for each remote host ssh
	var wg sync.WaitGroup
	for _, endpointName := range targetEndpoints {
		// Start go routine for specific host
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		go deployConfigs(&wg, semaphore, endpointName, hostsAndFilePaths[endpointName], hostsAndEndpointInfo[endpointName], commitFileInfo)
	}
	wg.Wait()

	// Save deployment errors to fail tracker
	if FailTracker != "" {
		err := recordDeploymentError(commitID)
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		return
	}

	// Show progress to user
	fmt.Printf("\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	fmt.Print("================================================\n")
}
