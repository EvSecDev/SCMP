// Package for bulk transferring files to and from remote hosts
package transfer

import (
	"context"
	"fmt"
	"os"
	"scmp/core/deployment/host"
	"scmp/internal/config"
	"scmp/internal/crypto"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/secrets"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"

	"golang.org/x/crypto/ssh"
)

func BulkFile(ctx context.Context, hostList map[str.RepoRootDir]config.EndpointInfo, sourceHost string, sourcePath string, destHost string, destPath string) (err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

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
			err = fmt.Errorf("failed to load file %s: %w", localFilePath, err)
			return
		}

		if len(fileBytes) == 0 {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Skipping file '%s', no data in file\n", localFilePath)
			continue
		}

		localFileContents[localFilePath] = fileBytes
		localFileHashes[localFilePath] = crypto.SHA256Sum(fileBytes)
	}

	var localToRemote [][]string

	for index := range localFilePaths {
		oneToOne := []string{localFilePaths[index], remoteFilePaths[index]}
		localToRemote = append(localToRemote, oneToOne)
	}

	for hostName := range cfg.HostInfo {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Host %s: Transferring files...\n", hostName)

		skipHost := parsing.CheckForOverride(ctx, destHost, string(hostName), hostList)
		if skipHost {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "    Host not desired\n")
			continue
		}

		// Retrieve host secrets
		cfg.HostInfo[hostName], err = secrets.GetHostValues(ctx, cfg.HostInfo[hostName])
		if err != nil {
			err = fmt.Errorf("error retrieving host secrets: %w", err)
			return
		}

		proxyName := cfg.HostInfo[hostName].Proxy
		if proxyName != "" {
			cfg.HostInfo[str.RepoRootDir(proxyName)], err = secrets.GetHostValues(ctx, cfg.HostInfo[str.RepoRootDir(proxyName)])
			if err != nil {
				err = fmt.Errorf("error retrieving proxy secrets: %w", err)
				return
			}
		}

		// Connect
		var hostMeta sshinternal.HostMeta
		hostMeta.Name = cfg.HostInfo[hostName].EndpointName
		hostMeta.Password = cfg.HostInfo[hostName].Password

		var proxyClient *ssh.Client
		hostMeta.SSHClient, proxyClient, err = sshinternal.ConnectToSSH(ctx, cfg.HostInfo[hostName], cfg.HostInfo[str.RepoRootDir(proxyName)])
		if err != nil {
			err = fmt.Errorf("failed connect to SSH server %w", err)
			return
		}
		defer func() {
			if proxyClient != nil {
				lerr := proxyClient.Close()
				if err == nil && lerr != nil {
					err = fmt.Errorf("proxy close: %w", lerr)
				}
			}
			lerr := hostMeta.SSHClient.Close()
			if err == nil && lerr != nil {
				err = fmt.Errorf("client close: %w", lerr)
			}
		}()

		err = host.RemoteDeploymentPreparation(ctx, &hostMeta)
		if err != nil {
			err = fmt.Errorf("host %s: remote system preparation failed: %w", hostName, err)
			return
		}

		// Transfer files - one to one mapping by index
		for _, transferFiles := range localToRemote {
			localFilePath := transferFiles[0]
			remoteFilePath := str.RemotePath(transferFiles[1])

			err = sshinternal.CreateRemoteFile(ctx, hostMeta, remoteFilePath, localFileContents[localFilePath], localFileHashes[localFilePath], "root:root", 644)
			if err != nil {
				err = fmt.Errorf("failed to transfer %s to remote path %s: %w", localFilePath, remoteFilePath, err)
				return
			}
		}

		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host %s: transfer complete.\n", hostName)

		host.CleanupRemote(ctx, hostMeta)
	}

	logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "All file transfers completed successfully\n")

	return
}
