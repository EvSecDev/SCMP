// controller
package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

func bulkFileTransfer(requestedHosts string, localFiles string, remoteFiles string) (err error) {
	if localFiles == "" || remoteFiles == "" {
		err = fmt.Errorf("must specific local and remote file(s)")
		return
	}

	localFilePaths := strings.Split(localFiles, ",")
	remoteFilePaths := strings.Split(remoteFiles, ",")

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

	for hostName, hostInfo := range config.hostInfo {
		printMessage(verbosityData, "  Host %s: Transferring files...\n", hostInfo.endpointName)

		skipHost := checkForOverride(requestedHosts, hostName)
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
		host.name = hostInfo.endpointName
		host.password = hostInfo.password
		host.transferBufferDir = hostInfo.remoteBufferDir
		host.backupPath = hostInfo.remoteBackupDir

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
			err = fmt.Errorf("Remote system preparation failed: %v", err)
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
	}

	printMessage(verbosityStandard, "All file transfers completed successfully\n")

	return
}
