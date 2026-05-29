package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"
)

// Adds dynamic values for the input HostMeta
// - OS type
// - Creates randomly named temporary transfer and backup directories
// - Sets strict permissions of login/runAs user for temp dirs
func RemoteDeploymentPreparation(ctx context.Context, host *sshinternal.HostMeta) (err error) {
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Determining remote OS\n", host.Name)

	command := sshinternal.BuildUnameKernel()
	unameOutput, err := command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("unable to determine OS, cannot deploy: %w", err)
		return
	}

	osName := strings.ToLower(unameOutput)
	if strings.Contains(osName, "bsd") {
		host.OSFamily = "bsd"
	} else if strings.Contains(osName, "linux") {
		host.OSFamily = "linux"
	} else {
		err = fmt.Errorf("received unknown os type: %s", unameOutput)
		host.OSFamily = "unknown"
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Preparing remote temporary directories\n", host.Name)

	// Random suffix
	buf := make([]byte, 16)
	_, err = rand.Read(buf)
	if err != nil {
		err = fmt.Errorf("failed to create random directory name: %w", err)
		return
	}
	mid := len(buf) / 2

	transferDirSuffix := hex.EncodeToString(buf[:mid])
	backupDirSuffix := hex.EncodeToString(buf[mid:])

	host.TransferBufferDir = str.RemotePath(RemoteTmpDir + "/scmp." + transferDirSuffix)
	host.BackupPath = str.RemotePath(RemoteTmpDir + "/scmp." + backupDirSuffix)

	// Create transfer and backup directory
	command = sshinternal.BuildMkdir(host.TransferBufferDir, host.BackupPath)
	command.DisableSudo = true
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed to setup remote temporary directories: %w", err)
		return
	}

	// Set stricter permissions
	command = sshinternal.BuildChmod(700, host.TransferBufferDir, host.BackupPath)
	command.DisableSudo = true
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed to change temporary directory permissions: %w", err)
		return
	}

	return
}
