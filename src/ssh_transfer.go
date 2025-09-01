// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

func entrySCP(commandname string, args []string) {
	var sourceHost string
	var sourcePath string
	var destHost string
	var destPath string

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	setDeployConfArguments(commandFlags)
	setGlobalArguments(commandFlags)

	commandFlags.Usage = func() {
		printHelpMenu(commandFlags, commandname, nil, "[srchost:]<srcpath> [dsthost:]<dstpath>", false)
	}
	if len(args) < 1 {
		printHelpMenu(commandFlags, commandname, nil, "[srchost:]<srcpath> [dsthost:]<dstpath>", false)
		os.Exit(1)
	}
	commandFlags.Parse(args[0:])

	remainingArgs := commandFlags.Args()

	source := remainingArgs[0]
	if strings.Contains(source, ":") {
		parts := strings.SplitN(source, ":", 2)
		sourceHost = parts[0]
		sourcePath = parts[1]
	} else {
		sourcePath = source
	}

	destination := remainingArgs[len(remainingArgs)-1]
	if strings.Contains(destination, ":") {
		parts := strings.SplitN(destination, ":", 2)
		destHost = parts[0]
		destPath = parts[1]
	} else {
		destPath = destination
	}

	err := config.extractOptions(config.filePath)
	logError("Error in controller configuration", err, true)

	err = bulkFileTransfer(sourceHost, sourcePath, destHost, destPath)
	logError("Failed to transfer files", err, false)
}

func bulkFileTransfer(sourceHost string, sourcePath string, destHost string, destPath string) (err error) {
	if sourcePath == "" || destPath == "" {
		err = fmt.Errorf("must specific source and destination path(s)")
		return
	}

	if sourceHost != "" {
		err = fmt.Errorf("remote to local scp is currently not supported")
		return
	}

	localFilePaths := strings.Split(sourcePath, ",")
	remoteFilePaths := strings.Split(destPath, ",")

	if len(localFilePaths) != len(remoteFilePaths) {
		err = fmt.Errorf("invalid length of local/remote files: lists must be equal length")
		return
	}

	localFileHashes := make(map[string]string)
	localFileContents := make(map[string][]byte)
	for _, localFilePath := range localFilePaths {
		var fileBytes []byte
		fileBytes, err = os.ReadFile(localFilePath)
		if err != nil {
			err = fmt.Errorf("failed to load file %s: %v", localFilePath, err)
			return
		}

		if len(fileBytes) == 0 {
			printMessage(verbosityStandard, "Skipping file '%s', no data in file\n", localFilePath)
			continue
		}

		localFileContents[localFilePath] = fileBytes
		localFileHashes[localFilePath] = SHA256Sum(fileBytes)
	}

	var localToRemote [][]string

	for index := range localFilePaths {
		oneToOne := []string{localFilePaths[index], remoteFilePaths[index]}
		localToRemote = append(localToRemote, oneToOne)
	}

	for hostName := range config.hostInfo {
		printMessage(verbosityData, "  Host %s: Transferring files...\n", hostName)

		skipHost := checkForOverride(destHost, hostName)
		if skipHost {
			printMessage(verbosityFullData, "    Host not desired\n")
			continue
		}

		// Retrieve host secrets
		config.hostInfo[hostName], err = retrieveHostSecrets(config.hostInfo[hostName])
		logError("Error retrieving host secrets", err, true)

		proxyName := config.hostInfo[hostName].proxy
		if proxyName != "" {
			config.hostInfo[proxyName], err = retrieveHostSecrets(config.hostInfo[proxyName])
			logError("Error retrieving proxy secrets", err, true)
		}

		// Connect
		var host HostMeta
		host.name = config.hostInfo[hostName].endpointName
		host.password = config.hostInfo[hostName].password

		var proxyClient *ssh.Client
		host.sshClient, proxyClient, err = connectToSSH(config.hostInfo[hostName], config.hostInfo[proxyName])
		if err != nil {
			err = fmt.Errorf("failed connect to SSH server %v", err)
			return
		}
		if proxyClient != nil {
			defer proxyClient.Close()
		}
		defer host.sshClient.Close()

		err = remoteDeploymentPreparation(&host)
		if err != nil {
			err = fmt.Errorf("Host %s: Remote system preparation failed: %v", hostName, err)
			return
		}

		// Transfer files - one to one mapping by index
		for _, transferFiles := range localToRemote {
			localFilePath := transferFiles[0]
			remoteFilePath := transferFiles[1]

			err = createRemoteFile(host, remoteFilePath, localFileContents[localFilePath], localFileHashes[localFilePath], "root:root", 644)
			if err != nil {
				err = fmt.Errorf("failed to transfer %s to remote path %s: %v", localFilePath, remoteFilePath, err)
				return
			}
		}

		printMessage(verbosityStandard, "  Host %s: transfer complete.\n", hostName)

		cleanupRemote(host)
	}

	printMessage(verbosityStandard, "All file transfers completed successfully\n")

	return
}
