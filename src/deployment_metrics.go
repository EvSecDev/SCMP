// controller
package main

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// Used for metrics - counting post deployment
type DeploymentMetrics struct {
	startTime       int64
	hostFiles       map[string][]string
	hostFilesMutex  sync.Mutex
	hostErr         map[string]string
	hostErrMutex    sync.Mutex
	fileErr         map[string]string
	fileErrMutex    sync.RWMutex
	fileAction      map[string]string
	fileActionMutex sync.Mutex
	hostBytes       map[string]int
	hostBytesMutex  sync.Mutex
	endTime         int64
}

// Summary of actions done and collected metrics
// Status could be UpToDate,Deployed,Partial,Failed
type DeploymentSummary struct {
	Status          string `json:"Status"`
	StartTime       string `json:"Start-Time"`
	EndTime         string `json:"End-Time"`
	ElapsedTime     string `json:"Elapsed-Time"`     // Human readable
	TransferredData string `json:"Transferred-Size"` // Human readable
	Counters        struct {
		Hosts          int `json:"Hosts" `
		Items          int `json:"Items"`
		CompletedHosts int `json:"Hosts-Completed"`
		CompletedItems int `json:"Items-Completed"`
		FailedHosts    int `json:"Hosts-Failed"`
		FailedItems    int `json:"Items-Failed"`
	} `json:"Counters"`
	CommitID string        `json:"Deployment-Commit-Hash"`
	Hosts    []HostSummary `json:"Hosts,omitempty"`
}

type HostSummary struct {
	Name            string        `json:"Name"`
	Status          string        `json:"Status,omitempty"`
	ErrorMsg        string        `json:"Error-Message,omitempty"`
	TotalItems      int           `json:"Total-Items,omitempty"`
	TransferredData string        `json:"Transferred-Size,omitempty"`
	Items           []ItemSummary `json:"Items,omitempty"`
}

type ItemSummary struct {
	Name     string `json:"Name"`
	Action   string `json:"Deployment-Action"`
	Status   string `json:"Status,omitempty"`
	ErrorMsg string `json:"Error-Message,omitempty"`
}

func (metric *DeploymentMetrics) addHostBytes(host string, deployedBytes int) {
	// Lock and write to metric var - increment total transferred bytes
	if deployedBytes > 0 {
		metric.hostBytesMutex.Lock()
		metric.hostBytes[host] += deployedBytes
		metric.hostBytesMutex.Unlock()
	}
}

func (metric *DeploymentMetrics) addFile(host string, allFileMeta map[string]FileInfo, files ...string) {
	metric.hostFilesMutex.Lock()
	metric.hostFiles[host] = append(metric.hostFiles[host], files...)
	metric.hostFilesMutex.Unlock()

	metric.fileActionMutex.Lock()
	for _, file := range files {
		metric.fileAction[file] = allFileMeta[file].action
	}
	metric.fileActionMutex.Unlock()
}

func (metric *DeploymentMetrics) addFileFailure(file string, err error) {
	if err == nil {
		return
	}

	// Ensure error string has no newlines
	message := err.Error()
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\r", " ")

	metric.fileErrMutex.Lock()
	metric.fileErr[file] = message
	metric.fileErrMutex.Unlock()
}

func (metric *DeploymentMetrics) addHostFailure(host string, err error) {
	if err == nil {
		return
	}

	// Ensure error string has no newlines
	message := err.Error()
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\r", " ")

	metric.hostErrMutex.Lock()
	metric.hostErr[host] = message
	metric.hostErrMutex.Unlock()
}

func (deployMetrics *DeploymentMetrics) createReport(commitID string) (deploymentSummary DeploymentSummary) {
	deploymentSummary.ElapsedTime = formatElapsedTime(deployMetrics)
	deploymentSummary.StartTime = convertMStoTimestamp(deployMetrics.startTime)
	deploymentSummary.EndTime = convertMStoTimestamp(deployMetrics.endTime)
	deploymentSummary.CommitID = commitID

	var allHostBytes int
	for _, bytes := range deployMetrics.hostBytes {
		allHostBytes += bytes
	}
	deploymentSummary.TransferredData = formatBytes(allHostBytes)

	deploymentSummary.Counters.Hosts = len(deployMetrics.hostFiles)

	for host, files := range deployMetrics.hostFiles {
		var hostSummary HostSummary
		hostSummary.Name = host
		hostSummary.ErrorMsg = deployMetrics.hostErr[host]
		hostSummary.TotalItems = len(files)

		if deploymentSummary.Counters.Hosts > 1 {
			hostSummary.TransferredData = formatBytes(deployMetrics.hostBytes[host])
		}

		deploymentSummary.Counters.Items += hostSummary.TotalItems

		var hostItemsDeployed int
		for _, file := range files {
			var fileSummary ItemSummary
			fileSummary.Name = file
			fileSummary.ErrorMsg = deployMetrics.fileErr[file]
			fileSummary.Action = deployMetrics.fileAction[file]

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
func (deploymentSummary DeploymentSummary) printFailures() (err error) {
	if deploymentSummary.Counters.FailedHosts == 0 && deploymentSummary.Counters.FailedItems == 0 {
		return
	}

	for _, hostDeployReport := range deploymentSummary.Hosts {
		if hostDeployReport.ErrorMsg != "" || hostDeployReport.Status == "Partial" || hostDeployReport.Status == "Failed" {
			printMessage(verbosityStandard, "Host: %s\n", hostDeployReport.Name)
		}

		if hostDeployReport.ErrorMsg != "" {
			printMessage(verbosityStandard, " Host Error: %s\n", hostDeployReport.ErrorMsg)
		}

		for _, fileDeployReport := range hostDeployReport.Items {
			fileErrorMessage := fileDeployReport.ErrorMsg
			if fileErrorMessage == "" {
				continue
			}

			printMessage(verbosityStandard, " File: '%s'\n", fileDeployReport.Name)

			// Print all the errors in a cascading format to show root cause
			errorLayers := strings.Split(fileErrorMessage, ": ")
			indentSpaces := 1
			for _, errorLayer := range errorLayers {
				// Print error at this layer with indent
				printMessage(verbosityStandard, "%s%s\n", strings.Repeat(" ", indentSpaces), errorLayer)

				// Increase indent for next line
				indentSpaces += 1
			}
		}
	}
	return
}

// Writes deployment summary to disk for deploy retry use
func (deploymentSummary DeploymentSummary) saveReport() (err error) {
	if deploymentSummary.Counters.FailedHosts == 0 && deploymentSummary.Counters.FailedItems == 0 {
		return
	}

	defer func() {
		// General warning on any err on return
		if err != nil {
			printMessage(verbosityStandard, "Warning: Recording of deployment failures encountered an error. Manual redeploy using '--deploy-failures' will not work.\n")
			printMessage(verbosityStandard, "  Please use the above errors to create a new commit with ONLY those failed files\n")
		}
	}()

	// Create JSON text
	deploymentSummaryJSON, err := json.MarshalIndent(deploymentSummary, "", " ")
	if err != nil {
		return
	}
	deploymentSummaryText := string(deploymentSummaryJSON)

	// Add FailTracker string to fail file
	failTrackerFile, err := os.Create(config.failTrackerFilePath)
	if err != nil {
		return
	}
	defer failTrackerFile.Close()

	deploymentSummaryText = deploymentSummaryText + "\n"

	// Always overwrite old contents
	_, err = failTrackerFile.WriteString(deploymentSummaryText)
	if err != nil {
		return
	}

	err = CreateJournaldLog(deploymentSummaryText, "err")
	if err != nil {
		return
	}

	return
}
