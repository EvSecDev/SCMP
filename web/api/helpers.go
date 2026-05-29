package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"scmp/core/filesystem"
	"scmp/internal/str"
	"scmp/web/internal"
	"strconv"
	"strings"
)

// Respond with a JSON-RPC error
func respondWithError(serverResponder http.ResponseWriter, id string, code int, message string, data string) {
	errObj := internal.Error{
		Code:    code,
		Message: message,
		Data:    data,
	}

	res := internal.Response{
		JSONRPC: "2.0",
		Error:   &errObj,
		ID:      id,
	}

	serverResponder.Header().Set("Content-Type", "application/json")
	serverResponder.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(serverResponder)
	_ = encoder.Encode(res)
}

func translateHeaderToWebMeta(filePath string, fileType string, lastModTime string, metadata filesystem.MetaHeader, fileData *[]byte) (webMeta FileMetadata) {
	webMeta.Path = filePath
	OwnerGroup := strings.Split(metadata.TargetFileOwnerGroup, ":")
	if len(OwnerGroup) == 2 {
		webMeta.OwnerName = OwnerGroup[0]
		webMeta.GroupName = OwnerGroup[1]
	} else {
		// No error, might be processing a local-only file with no actual metadata
		webMeta.OwnerName = "root"
		webMeta.GroupName = "root"
	}
	if metadata.TargetFilePermissions == 0 {
		// Local only file, use default
		if fileType == "directory" {
			webMeta.Permissions = "750"
		} else {
			webMeta.Permissions = "640"
		}
	} else {
		webMeta.Permissions = strconv.Itoa(metadata.TargetFilePermissions)
	}
	webMeta.Type = fileType
	if fileData != nil {
		webMeta.Size = len(*fileData)
	} else {
		webMeta.Size = 0
	}
	webMeta.LastModified = lastModTime
	webMeta.ReloadGroup = metadata.ReloadGroup
	webMeta.ExternalContentLocation = metadata.ExternalContentLocation
	webMeta.Dependencies = metadata.Dependencies
	webMeta.PreDeployCommands = metadata.PreDeployCommands
	webMeta.InstallCommands = metadata.InstallCommands
	webMeta.CheckCommands = metadata.CheckCommands
	webMeta.ReloadCommands = metadata.ReloadCommands
	return
}

func translateWebMetaToHeader(webMeta FileMetadata) (metadata filesystem.MetaHeader, err error) {
	permissions, err := strconv.Atoi(webMeta.Permissions)
	if err != nil {
		err = fmt.Errorf("invalid permission numeric: %w", err)
		return
	}
	metadata.TargetFilePermissions = permissions
	metadata.TargetFileOwnerGroup = webMeta.OwnerName + ":" + webMeta.GroupName
	metadata.ExternalContentLocation = webMeta.ExternalContentLocation
	metadata.SymbolicLinkTarget = str.RemotePath(webMeta.SymbolicLinkTarget)
	metadata.Dependencies = webMeta.Dependencies
	metadata.PreDeployCommands = webMeta.PreDeployCommands
	metadata.InstallCommands = webMeta.InstallCommands
	metadata.CheckCommands = webMeta.CheckCommands
	metadata.ReloadCommands = webMeta.ReloadCommands
	metadata.ReloadGroup = webMeta.ReloadGroup
	return
}
