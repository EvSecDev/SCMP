// controller
package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"syscall"
)

// Checks for active network interfaces (can't deploy to remote endpoints if no network)
func localSystemChecks() (err error) {
	printMessage(verbosityProgress, "Running local system checks...\n")
	printMessage(verbosityProgress, "  Ensuring system has an active network interface\n")

	// Get list of local systems network interfaces
	systemNetInterfaces, err := net.Interfaces()
	if err != nil {
		err = fmt.Errorf("failed to obtain system network interfaces: %v", err)
		return
	}

	// Ensure system has an active network interface
	var noActiveNetInterface bool
	for _, iface := range systemNetInterfaces {
		// Net interface is up
		if iface.Flags&net.FlagUp != 0 {
			noActiveNetInterface = false
			break
		}
		noActiveNetInterface = true
	}
	if noActiveNetInterface {
		err = fmt.Errorf("no active network interfaces found, will not attempt network connections")
		return
	}

	return
}

// Print out deployment information in dry run mode
func printDeploymentInformation(commitFileInfo map[string]FileInfo, allDeploymentHosts []string) {
	// Notify user that program is in dry run mode
	printMessage(verbosityStandard, "Requested dry-run, aborting deployment\n")
	printMessage(verbosityStandard, "Outputting information collected for deployment:\n")

	// Print deployment info by host
	for _, endpointName := range allDeploymentHosts {
		hostInfo := config.hostInfo[endpointName]
		printHostInformation(hostInfo)
		printMessage(verbosityStandard, "  Files:\n")

		// Identify maximum indent file name prints will need to be
		var maxFileNameLength int
		var maxActionLength int
		for _, filePath := range hostInfo.deploymentList.files {
			// Format to remote path type
			_, targetFile := translateLocalPathtoRemotePath(filePath)

			nameLength := len(targetFile)
			if nameLength > maxFileNameLength {
				maxFileNameLength = nameLength
			}

			actionLength := len(commitFileInfo[filePath].action)
			if actionLength > maxActionLength {
				maxActionLength = actionLength
			}
		}
		// Increment indent so longest name has at least some space after it
		maxFileNameLength += 1
		maxActionLength += 9

		// Print out files for this specific host
		for _, file := range hostInfo.deploymentList.files {
			// Format to remote path type
			_, targetFile := translateLocalPathtoRemotePath(file)

			// Determine how many spaces to add after file name
			fileIndentSpaces := maxFileNameLength - len(targetFile)

			// Determine how many spaces to add after action name
			actionIndentSpaces := maxActionLength - len(commitFileInfo[file].action)

			// Print what we are going to do, the local file path, and remote file path
			printMessage(verbosityStandard, "       %s:%s%s%s# %s\n", commitFileInfo[file].action, strings.Repeat(" ", actionIndentSpaces), targetFile, strings.Repeat(" ", fileIndentSpaces), file)
		}
	}
}

// Ties into dry-runs to have a unified print of host information
func printHostInformation(hostInfo EndpointInfo) {
	// Print out information for this specific host
	printMessage(verbosityStandard, "Host: %s\n", hostInfo.endpointName)
	printMessage(verbosityStandard, "  Options:\n")
	printMessage(verbosityStandard, "       Endpoint Address:  %s\n", hostInfo.endpoint)
	printMessage(verbosityStandard, "       SSH User:          %s\n", hostInfo.endpointUser)
}

// Runs user defined commands locally
// If err is present on return, deployment should fail
// deploy metrics used to track any other failures
func runPreDeploymentCommands(deployMetrics *DeploymentMetrics, hostname string, deploymentList DeploymentList, allFileMeta map[string]FileInfo, allFileData map[string][]byte) (err error) {
	const userReqStdinPrefix string = "writefileinto:" // User decides which command gets file content written to stdin (optional)

	for _, repoFilePath := range deploymentList.files {
		if !allFileMeta[repoFilePath].predeployRequired {
			continue
		}

		printMessage(verbosityProgress, "Host %s: Running pre-deployment commands for file '%s'\n", hostname, repoFilePath)

		for _, predeployCommand := range allFileMeta[repoFilePath].predeploy {
			var writeConfToStdin bool
			if strings.HasPrefix(predeployCommand, userReqStdinPrefix) {
				writeConfToStdin = true
				predeployCommand = strings.TrimPrefix(predeployCommand, userReqStdinPrefix)
			}

			printMessage(verbosityData, "Host %s:   Running pre-deployment command '%s'\n", hostname, predeployCommand)

			// Separate binary from arguments
			commandArgs := strings.Fields(predeployCommand)
			commandExe := commandArgs[0]

			cmd := exec.Command(commandExe, commandArgs[1:]...)

			var stderrBuf bytes.Buffer
			cmd.Stderr = &stderrBuf

			var stdin io.WriteCloser
			if writeConfToStdin {
				stdin, err = cmd.StdinPipe()
				if err != nil {
					err = fmt.Errorf("error creating stdin writer: %v", err)
					return
				}
			}

			// Run the command
			err = cmd.Start()
			if err != nil {
				err = fmt.Errorf("error starting command: %v", err)
				return
			}

			if writeConfToStdin {
				// Write files contents to stdin if requested
				hashIndex := allFileMeta[repoFilePath].hash
				_, err = stdin.Write(allFileData[hashIndex])
				if err != nil {
					err = fmt.Errorf("failed to write stdin to command: %v", err)
					return
				}

				err = stdin.Close()
				if err != nil {
					err = fmt.Errorf("failed to close stdin: %v", err)
					return
				}
			}

			// Wait for command to exit
			err = cmd.Wait()
			if err == nil {
				// Exit code 0 - next cmd
				continue
			}

			if exitErr, ok := err.(*exec.ExitError); ok {
				if _, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					// Parsable exit status - command failed externally (non-zero)
					err = fmt.Errorf("predeploy command '%s': %v: %s", cmd.String(), err, stderrBuf.String())

					// Add to fail metrics - will trigger skip deployment of it and any related
					deployMetrics.addFileFailure(repoFilePath, err)
				} else {
					// Unparsable exit status (maybe Windows) - fail host deployment
					err = fmt.Errorf("failed to evaluate exit status of command '%s': %v", cmd.String(), err)
					return
				}
			} else {
				// Failed due to local issue, fail host deployment
				err = fmt.Errorf("error running command '%s': %v", cmd.String(), err)
				return
			}
		}
	}

	return
}
