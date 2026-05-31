// Package for all pre-deploy logic
package predeploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"scmp/core/deployment"
	"scmp/core/deployment/metrics"
	"scmp/internal/config"
	"scmp/internal/crypto"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
	"syscall"
)

// Print out deployment information in dry run mode
func PrintDeploymentInformation(ctx context.Context, deployFiles *deployment.AllFiles, allDeploymentHosts []str.RepoRootDir, hostFiles map[str.RepoRootDir]*deployment.HostFiles) {
	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	// Notify user that program is in dry run mode
	logctx.LogStdInfo(ctx, "Requested dry-run, aborting deployment\n")
	logctx.LogStdInfo(ctx, "Outputting information collected for deployment:\n")

	// Print deployment info by host
	for _, endpointName := range allDeploymentHosts {

		hostInfo := config.HostInfo[endpointName]
		PrintHostInformation(ctx, hostInfo)
		logctx.LogStdInfo(ctx, "  Files:\n")

		deploymentList := hostFiles[endpointName]

		// Identify maximum indent file name prints will need to be
		var maxFileNameLength int
		var maxActionLength int
		for _, independentDeploymentList := range deploymentList.Groups {
			for _, filePath := range independentDeploymentList.GetOrderedList() {
				// Format to remote path type
				_, targetFile := translateLocalPathtoRemotePath(ctx, filePath)

				nameLength := len(targetFile)
				if nameLength > maxFileNameLength {
					maxFileNameLength = nameLength
				}

				info := deployFiles.GetFileInfo(filePath)
				actionLength := len(info.Action)
				if actionLength > maxActionLength {
					maxActionLength = actionLength
				}
			}
		}
		// Increment indent so longest name has at least some space after it
		maxFileNameLength += 1
		maxActionLength += 9

		// Print out files for this specific host
		for _, independentDeploymentList := range deploymentList.Groups {
			for _, file := range independentDeploymentList.GetOrderedList() {
				// Format to remote path type
				_, targetFile := translateLocalPathtoRemotePath(ctx, file)

				// Determine how many spaces to add after file name
				fileIndentSpaces := maxFileNameLength - len(targetFile)

				info := deployFiles.GetFileInfo(file)

				// Determine how many spaces to add after action name
				actionIndentSpaces := maxActionLength - len(info.Action)

				// Print what we are going to do, the local file path, and remote file path
				logctx.LogStdInfo(ctx, "       %s:%s%s%s# %s\n",
					info.Action, strings.Repeat(" ", actionIndentSpaces), targetFile, strings.Repeat(" ", fileIndentSpaces), file)
			}
		}
	}
}

// Ties into dry-runs to have a unified print of host information
func PrintHostInformation(ctx context.Context, hostInfo config.EndpointInfo) {
	// Print out information for this specific host
	var infoOutput string
	infoOutput += fmt.Sprintf("Host: %s\n", hostInfo.EndpointName)
	infoOutput += ("  Options:\n")
	infoOutput += fmt.Sprintf("       Endpoint Address:  %s\n", hostInfo.Endpoint)
	infoOutput += fmt.Sprintf("       SSH User:          %s\n", hostInfo.EndpointUser)
	logctx.LogStdInfo(ctx, "%s\n", infoOutput)
}

// Runs user defined commands locally
// If err is present on return, deployment should fail
// deploy metrics used to track any other failures
func RunPreDeploymentCommands(ctx context.Context, deployMetrics *metrics.Metrics, hostname str.RepoRootDir, files *deployment.HostFiles) (err error) {
	// Optional user markers for stdin/stdout append/overwrite
	const reqStdinMacro string = "<<<{@LOCALFILEDATA}"
	const reqStdoutApSuffix string = ">>{@REMOTEFILEDATA}"
	const reqStdoutOwSuffix string = ">{@REMOTEFILEDATA}"

	for _, independentDeploymentList := range files.Groups {
		for _, repoFilePath := range independentDeploymentList.GetOrderedList() {
			repoFileInfo := files.GetFileInfo(repoFilePath)

			if !repoFileInfo.PredeployRequired {
				continue
			}

			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Running pre-deployment commands for file '%s'\n", hostname, repoFilePath)

			for _, predeployCommand := range repoFileInfo.Predeploy {
				// Avoid stdin/stdout markers from being ignored due to lingering spaces
				predeployCommand = strings.TrimSpace(predeployCommand)

				oldHashIndex := repoFileInfo.Hash

				var writeConfToStdin bool
				if strings.Contains(predeployCommand, reqStdinMacro) {
					writeConfToStdin = true
					predeployCommand = strings.ReplaceAll(predeployCommand, reqStdinMacro, "")
				}

				var writeStdoutToFile bool
				var appendStdoutToFile bool
				if strings.HasSuffix(predeployCommand, reqStdoutOwSuffix) {
					writeStdoutToFile = true
					predeployCommand = strings.TrimSuffix(predeployCommand, reqStdoutOwSuffix)
				} else if strings.HasSuffix(predeployCommand, reqStdoutApSuffix) {
					appendStdoutToFile = true
					predeployCommand = strings.TrimSuffix(predeployCommand, reqStdoutApSuffix)
				}

				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Running pre-deployment command '%s'\n", hostname, predeployCommand)

				// Retrieve executable name
				fields := strings.Fields(predeployCommand)
				commandExe := fields[0]

				// Evaluate quoting to retrieve distinct arguments
				var commandArgs []string
				newArgs := strings.Replace(predeployCommand, commandExe, "", 1)
				commandArgs, err = parsing.HandleQuotedArgs(newArgs)
				if err != nil {
					err = fmt.Errorf("error evaluating quoting in command: %w", err)
					return
				}

				cmd := exec.Command(commandExe, commandArgs...)

				var stdoutBuf bytes.Buffer
				if writeStdoutToFile || appendStdoutToFile {
					cmd.Stdout = &stdoutBuf
				}

				var stderrBuf bytes.Buffer
				cmd.Stderr = &stderrBuf

				var stdin io.WriteCloser
				if writeConfToStdin {
					stdin, err = cmd.StdinPipe()
					if err != nil {
						err = fmt.Errorf("error creating stdin writer: %w", err)
						return
					}
				}

				// Run the command
				err = cmd.Start()
				if err != nil {
					err = fmt.Errorf("error starting command: %w", err)
					return
				}

				if writeConfToStdin {
					// Write files contents to stdin if requested
					_, err = stdin.Write(files.GetFileData(oldHashIndex))
					if err != nil {
						err = fmt.Errorf("failed to write stdin to command: %w", err)
						return
					}

					err = stdin.Close()
					if err != nil {
						err = fmt.Errorf("failed to close stdin: %w", err)
						return
					}
				}

				// Wait for command to exit
				err = cmd.Wait()
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						if _, ok := exitErr.Sys().(syscall.WaitStatus); ok {
							// Parsable exit status - command failed externally (non-zero)
							err = fmt.Errorf("pre-deploy command '%s': %w: %s", cmd.String(), err, stderrBuf.String())

							// Add to fail metrics - will trigger skip deployment of it and any related
							deployMetrics.AddFileFailure(hostname, repoFilePath, err)
							continue
						} else {
							// Unparsable exit status (maybe Windows) - fail host deployment
							err = fmt.Errorf("failed to evaluate exit status of command '%s': %w", cmd.String(), err)
							return
						}
					} else {
						// Failed due to local issue, fail host deployment
						err = fmt.Errorf("error running command '%s': %w", cmd.String(), err)
						return
					}
				}

				// Handle content modifications if requested
				if writeStdoutToFile {
					// Have to rehash contents to prevent clobbering identical input files for other hosts
					newHashIndex := str.FileID(crypto.SHA256Sum(stdoutBuf.Bytes()))
					files.StoreDataOnce(newHashIndex, stderrBuf.Bytes())

					// Change hash pointer to new contents
					files.ChangeFileDataPointer(repoFilePath, newHashIndex)
				} else if appendStdoutToFile {
					existingFileContent := files.GetFileData(oldHashIndex)

					// If content doesn't end with newline, add one for proper append behavior
					if !strings.HasSuffix(string(existingFileContent), "\n") {
						existingFileContent = append(existingFileContent, '\n')
					}

					// Add script output to contents
					newFileContent := append(existingFileContent, stdoutBuf.Bytes()...)

					// Rehash and add to content map
					newHashIndex := str.FileID(crypto.SHA256Sum(newFileContent))
					files.StoreDataOnce(newHashIndex, newFileContent)

					// Change hash pointer to new contents
					files.ChangeFileDataPointer(repoFilePath, newHashIndex)
				}
			}
		}
	}

	return
}
