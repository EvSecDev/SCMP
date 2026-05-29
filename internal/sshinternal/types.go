// Package for all low level interaction with SSH servers
package sshinternal

import (
	"scmp/internal/str"

	"golang.org/x/crypto/ssh"
)

// Type for commands run remotely
type RemoteCommand struct {
	Raw          string // Command string
	RunAsUser    string // Username to run command as (only with sudo)
	DisableSudo  bool   // Run command with privileges (as login user)
	Timeout      int    // In seconds
	StreamStdout bool   // Progressively stream output of command to stdout of this program (almost always false)
}

// Struct for remote file metadata
type RemoteFileInfo struct {
	Hash        str.FileID
	Name        str.RemotePath
	FsType      string
	Permissions int
	Owner       string
	Group       string
	Size        int
	LinkTarget  str.RemotePath
	Exists      bool
}

// Deployment host metadata to easily pass between SSH functions
type HostMeta struct {
	Name              str.RepoRootDir
	OSFamily          string
	Password          string
	SSHClient         *ssh.Client
	TransferBufferDir str.RemotePath
	BackupPath        str.RemotePath
}
