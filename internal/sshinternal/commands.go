package sshinternal

import (
	"scmp/internal/str"
	"strconv"
	"strings"
)

// Constructors for remote SSH commands
// Standardizes command names and their arguments

func BuildUnameKernel() (remoteCommand RemoteCommand) {
	const unameCmd string = "uname -s"
	remoteCommand.Raw = unameCmd
	remoteCommand.DisableSudo = true
	remoteCommand.Timeout = DefaultRemoteCommandTimeout
	return
}

func BuildStat(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	// Fixed output for extractMetadataFromStat function parsing
	const statCmd string = "stat --format='[%n],[%F],[%U],[%G],[%a],[%s],[%N]' "
	remoteCommand.Raw = statCmd + "'" + string(remotePath) + "'"
	remoteCommand.Timeout = DefaultRemoteCommandTimeout
	return
}

func BuildBSDStat(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	// Fixed output for extractMetadataFromStat function parsing
	const statBsdCmd string = "stat -f '[%N],[%HT],[%Su],[%Sg],[%Lp],[%z],[target=%Y]' "
	remoteCommand.Raw = statBsdCmd + "'" + string(remotePath) + "'"
	remoteCommand.Timeout = DefaultRemoteCommandTimeout
	return
}

func BuildLs(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const lsCmd string = "ls -A "
	remoteCommand.Raw = lsCmd + "'" + string(remotePath) + "'"
	remoteCommand.Timeout = DefaultRemoteCommandTimeout
	return
}

func BuildLsList(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const lsNamesCmd string = "ls -1AF "
	remoteCommand.Raw = lsNamesCmd + "'" + string(remotePath) + "'"
	remoteCommand.Timeout = 15
	return
}

func BuildHashCmd(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const hashCmd string = "sha256sum "
	remoteCommand.Raw = hashCmd + "'" + string(remotePath) + "'"
	remoteCommand.Timeout = 90
	return
}

func BuildMv(srcRemotePath str.RemotePath, dstRemotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const mvCmd string = "mv "
	remoteCommand.Raw = mvCmd + "'" + string(srcRemotePath) + "' '" + string(dstRemotePath) + "'"
	remoteCommand.Timeout = 90
	return
}

func BuildCp(srcRemotePath str.RemotePath, dstRemotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const cpCmd string = "cp -p "
	remoteCommand.Raw = cpCmd + "'" + string(srcRemotePath) + "' '" + string(dstRemotePath) + "'"
	remoteCommand.Timeout = 90
	return
}

func BuildMkdir(remotePaths ...str.RemotePath) (remoteCommand RemoteCommand) {
	const mkdirCmd string = "mkdir -p "

	var requestedPaths []string
	for _, remotePath := range remotePaths {
		requestedPaths = append(requestedPaths, "'"+string(remotePath)+"'")
	}
	dirsToCreate := strings.Join(requestedPaths, " ")

	remoteCommand.Raw = mkdirCmd + dirsToCreate
	remoteCommand.Timeout = 30
	return
}

func BuildChown(ownerGroup string, remotePaths ...str.RemotePath) (remoteCommand RemoteCommand) {
	const chownCmd string = "chown "

	var requestedPaths []string
	for _, remotePath := range remotePaths {
		requestedPaths = append(requestedPaths, "'"+string(remotePath)+"'")
	}
	itemsToChown := strings.Join(requestedPaths, " ")

	remoteCommand.Raw = chownCmd + "'" + ownerGroup + "' " + itemsToChown
	remoteCommand.Timeout = 20
	return
}

func BuildChmod(permissionBits int, remotePaths ...str.RemotePath) (remoteCommand RemoteCommand) {
	const chmodCmd string = "chmod "
	permissionString := strconv.Itoa(permissionBits)

	var requestedPaths []string
	for _, remotePath := range remotePaths {
		requestedPaths = append(requestedPaths, "'"+string(remotePath)+"'")
	}
	itemsToChmod := strings.Join(requestedPaths, " ")

	remoteCommand.Raw = chmodCmd + "'" + permissionString + "' " + itemsToChmod
	remoteCommand.Timeout = 20
	return
}

func BuildRm(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const rmCmd string = "rm "
	remoteCommand.Raw = rmCmd + "'" + string(remotePath) + "'"
	remoteCommand.Timeout = 15
	return
}

func BuildRmAll(remotePaths ...str.RemotePath) (remoteCommand RemoteCommand) {
	const rmAllCmd string = "rm -r "

	// Concat variable input paths together
	var requestedPaths []string
	for _, remotePath := range remotePaths {
		requestedPaths = append(requestedPaths, "'"+string(remotePath)+"'")
	}
	itemsToRemove := strings.Join(requestedPaths, " ")

	remoteCommand.Raw = rmAllCmd + itemsToRemove
	remoteCommand.Timeout = 90
	return
}

func BuildRmdir(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const rmdirCmd string = "rmdir "
	remoteCommand.Raw = rmdirCmd + "'" + string(remotePath) + "'"
	remoteCommand.Timeout = DefaultRemoteCommandTimeout
	return
}

func BuildLink(linkTarget str.RemotePath, linkName str.RemotePath) (remoteCommand RemoteCommand) {
	const lnCmd string = "ln -snf "
	remoteCommand.Raw = lnCmd + "'" + string(linkTarget) + "' '" + string(linkName) + "'"
	remoteCommand.Timeout = DefaultRemoteCommandTimeout
	return
}

func BuildTouch(remotePath str.RemotePath) (remoteCommand RemoteCommand) {
	const touchCmd string = "touch"
	remoteCommand.Raw = touchCmd + " '" + string(remotePath) + "'"
	remoteCommand.Timeout = DefaultRemoteCommandTimeout
	return
}
