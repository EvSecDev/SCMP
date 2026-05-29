package metrics

import (
	"context"
	"encoding/json"
	"os"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"strings"
)

func (metric *Metrics) CreateReport(commitID string) (deploymentSummary Summary) {
	deploymentSummary.ElapsedTime = parsing.FormatElapsedTime(metric.startTime.UnixMilli(), metric.endTime.UnixMilli())
	deploymentSummary.StartTime = parsing.ConvertMStoTimestamp(metric.startTime.UnixMilli())
	deploymentSummary.EndTime = parsing.ConvertMStoTimestamp(metric.endTime.UnixMilli())
	deploymentSummary.CommitID = commitID

	var allHostBytes int
	for _, bytes := range metric.hostBytes {
		allHostBytes += bytes
	}
	deploymentSummary.TransferredData = parsing.FormatBytes(allHostBytes)

	deploymentSummary.Counters.Hosts = len(metric.hostFiles)

	for host, files := range metric.hostFiles {
		var hostSummary HostSummary
		hostSummary.Name = host
		err, hasErr := metric.hostErr[host]
		if hasErr {
			hostSummary.ErrorMsg = err.Error()
			hostSummary.ErrorMsg = strings.ReplaceAll(hostSummary.ErrorMsg, "\n", ": ")
			hostSummary.ErrorMsg = strings.ReplaceAll(hostSummary.ErrorMsg, "\r", ": ")
		}
		hostSummary.TotalItems = len(files)

		if deploymentSummary.Counters.Hosts > 1 {
			hostSummary.TransferredData = parsing.FormatBytes(metric.hostBytes[host])
		}

		deploymentSummary.Counters.Items += hostSummary.TotalItems

		hostFileErrs := metric.hostsFileErr[host]

		var hostItemsDeployed int
		for _, file := range files {
			var fileSummary ItemSummary
			fileSummary.Name = file
			err, hasErr := hostFileErrs[file]
			if hasErr {
				fileSummary.ErrorMsg = err.Error()
				fileSummary.ErrorMsg = strings.ReplaceAll(fileSummary.ErrorMsg, "\n", ": ")
				fileSummary.ErrorMsg = strings.ReplaceAll(fileSummary.ErrorMsg, "\r", ": ")
			}
			fileSummary.Action = metric.fileAction[file]

			if fileSummary.ErrorMsg != "" {
				// Individual file failure
				fileSummary.Status = "Failed"
				deploymentSummary.Counters.FailedItems++
			} else if hostSummary.ErrorMsg != "" {
				// Entire host failures indicate every file failed
				fileSummary.Status = "Failed"
				deploymentSummary.Counters.FailedItems++
			} else {
				// No file errors indicate it was deployed
				fileSummary.Status = "Deployed"
				hostItemsDeployed++
				deploymentSummary.Counters.CompletedItems++
			}

			hostSummary.Items = append(hostSummary.Items, fileSummary)
		}

		if hostItemsDeployed == hostSummary.TotalItems {
			// If all items were successful, whole host deploy was successful
			hostSummary.Status = "Deployed"
			deploymentSummary.Counters.CompletedHosts++
		} else if hostItemsDeployed > 0 {
			// If at least one file deployed, host is partially successful
			hostSummary.Status = "Partial"
			deploymentSummary.Counters.FailedHosts++
		} else if hostItemsDeployed == 0 {
			// No successful files, whole host marked failed
			hostSummary.Status = "Failed"
			deploymentSummary.Counters.FailedHosts++
		} else {
			// Catch all
			hostSummary.Status = "Unknown"
			deploymentSummary.Counters.FailedHosts++
		}

		deploymentSummary.Hosts = append(deploymentSummary.Hosts, hostSummary)
	}

	if deploymentSummary.Counters.CompletedHosts == deploymentSummary.Counters.Hosts {
		deploymentSummary.Status = "Deployed"
	} else if deploymentSummary.Counters.CompletedHosts > 0 && deploymentSummary.Counters.FailedHosts > 0 {
		deploymentSummary.Status = "Partial"
	} else if deploymentSummary.Counters.CompletedHosts == 0 && deploymentSummary.Counters.FailedHosts > 0 {
		deploymentSummary.Status = "Failed"
	} else if deploymentSummary.Counters.Hosts == 0 {
		deploymentSummary.Status = "UpToDate"
	} else {
		deploymentSummary.Status = "Unknown"
	}

	return
}

// Prints custom stdout to user to show the root-cause errors
func (deploymentSummary Summary) PrintFailures(ctx context.Context) (err error) {
	if deploymentSummary.Counters.FailedHosts == 0 && deploymentSummary.Counters.FailedItems == 0 {
		return
	}

	for _, hostDeployReport := range deploymentSummary.Hosts {
		if hostDeployReport.ErrorMsg != "" || hostDeployReport.Status == "Partial" || hostDeployReport.Status == "Failed" {
			logctx.LogStdInfo(ctx, "Host: %s\n", hostDeployReport.Name)
		}

		if hostDeployReport.ErrorMsg != "" {
			logctx.LogStdInfo(ctx, " Host Error: %s\n", hostDeployReport.ErrorMsg)
		}

		for _, fileDeployReport := range hostDeployReport.Items {
			fileErrorMessage := fileDeployReport.ErrorMsg
			if fileErrorMessage == "" {
				continue
			}

			logctx.LogStdInfo(ctx, " File: '%s'\n", fileDeployReport.Name)

			// Print all the errors in a cascading format to show root cause
			errorLayers := strings.Split(fileErrorMessage, ": ")
			indentSpaces := 1
			for _, errorLayer := range errorLayers {
				// Print error at this layer with indent
				logctx.LogStdInfo(ctx, "%s%s\n", strings.Repeat(" ", indentSpaces), errorLayer)

				// Increase indent for next line
				indentSpaces += 1
			}
		}
	}
	return
}

// Writes deployment summary to disk for deploy retry use
func (deploymentSummary Summary) SaveReport(ctx context.Context, filePath string) (err error) {
	if deploymentSummary.Counters.FailedHosts == 0 && deploymentSummary.Counters.FailedItems == 0 {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "No failures to save (no failed hosts and no failed items)\n")
		return
	}

	defer func() {
		// General warning on any err on return
		if err != nil {
			logctx.LogStdWarn(ctx, "Recording of deployment failures encountered an error. Manual redeploy using '--deploy-failures' will not work.\n")
			logctx.LogStdWarn(ctx, "  Please use the above errors to create a new commit with ONLY those failed files\n")
		}
	}()

	// Create JSON text
	deploymentSummaryJSON, err := json.MarshalIndent(deploymentSummary, "", " ")
	if err != nil {
		return
	}
	deploymentSummaryText := string(deploymentSummaryJSON)

	// Add FailTracker string to fail file
	failTrackerFile, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer func() {
		lerr := failTrackerFile.Close()
		if err == nil && lerr != nil {
			err = lerr
		}
	}()

	deploymentSummaryText = deploymentSummaryText + "\n"

	// Always overwrite old contents
	_, err = failTrackerFile.WriteString(deploymentSummaryText)
	if err != nil {
		return
	}

	return
}
