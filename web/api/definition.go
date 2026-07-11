package api

import (
	"reflect"
	"scmp/web/internal"
)

// Define API endpoints and their specific methods, request/reply, and handler functions
func SetupAPIEndpoints() (endpoints map[string]internal.Catalog) {
	endpoints = make(map[string]internal.Catalog)

	// ====================== DOCS ======================
	apiDocs := internal.Catalog{
		Name:               "API Browser",
		Description:        "Retrieve full definition for this API",
		Method:             "api.browser",
		Params:             nil,
		Result:             reflect.TypeOf([]any{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    handleAPIBrowser,
	}
	endpoints[apiDocs.Method] = apiDocs

	// ====================== AUTH ======================
	login := internal.Catalog{
		Name:               "Login Request",
		Description:        "Submit username and password to retrieve token",
		Method:             "user.login",
		Params:             reflect.TypeOf(UserLogin{}),
		Result:             reflect.TypeOf(AuthToken{}),
		AllowedPermissions: []internal.PermissionAction{internal.NoAuthAction},
		HandlerFunction:    loginAPI,
	}
	endpoints[login.Method] = login

	logout := internal.Catalog{
		Name:               "Logout Request",
		Description:        "Revoke user ID token",
		Method:             "user.logout",
		Params:             reflect.TypeOf(AuthToken{}),
		Result:             reflect.TypeOf(UserLogout{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    logoutAPI,
	}
	endpoints[logout.Method] = logout

	// ====================== SETTINGS ======================

	verInfo := internal.Catalog{
		Name:               "Version Information",
		Description:        "Retrieves versioning and program metadata",
		Method:             "settings.info.version",
		Params:             nil,
		Result:             reflect.TypeOf(VersionInfo{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    versionAPI,
	}
	endpoints[verInfo.Method] = verInfo

	repoList := internal.Catalog{
		Name:               "Repo List",
		Description:        "Gets list of repositories for given user",
		Method:             "settings.repositories.list",
		Params:             nil,
		Result:             reflect.TypeOf(RepoList{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    repoListAPI,
	}
	endpoints[repoList.Method] = repoList

	hostList := internal.Catalog{
		Name:               "Host List",
		Description:        "Gets list of remote hosts for repository",
		Method:             "settings.hosts.list",
		Params:             reflect.TypeOf(HostListReq{}),
		Result:             reflect.TypeOf(HostList{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    hostListAPI,
	}
	endpoints[hostList.Method] = hostList

	// ====================== INTERFACE ======================

	promptAnswer := internal.Catalog{
		Name:               "Answer a prompt",
		Description:        "Send Answer to a pending prompt",
		Method:             "user.pending.prompt.answer",
		Params:             reflect.TypeOf([]PromptAnswer{}),
		Result:             reflect.TypeOf(NilSuccess{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    answerPendingPrompts,
	}
	endpoints[promptAnswer.Method] = promptAnswer

	// ====================== DIR LIST ======================
	filesList := internal.Catalog{
		Name:               "Directory List",
		Description:        "Gets items under a directory with their basic metadata",
		Method:             "fs.directory.list",
		Params:             reflect.TypeOf(PathRequest{}),
		Result:             reflect.TypeOf([]FileMetadata{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    dirListAPI,
	}
	endpoints[filesList.Method] = filesList

	// ====================== FILE DATA/METADATA ======================

	fileDataGet := internal.Catalog{
		Name:               "Data Download Link",
		Description:        "Retrieves link to raw file download (excludes header)",
		Method:             "fs.item.data.download",
		Params:             reflect.TypeOf(PathRequest{}),
		Result:             reflect.TypeOf(DownloadLink{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    contentReadAPI,
	}
	endpoints[fileDataGet.Method] = fileDataGet

	fileDataPut := internal.Catalog{
		Name:               "Data Upload Link",
		Description:        "Retrieves link to raw file upload (excluding header)",
		Method:             "fs.item.data.save",
		Params:             reflect.TypeOf(ProcessUploadReq{}),
		Result:             reflect.TypeOf(NilSuccess{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    contentEditAPI,
	}
	endpoints[fileDataPut.Method] = fileDataPut

	fileMetaGet := internal.Catalog{
		Name:               "File Metadata Read",
		Description:        "Retrieves file metadata header",
		Method:             "fs.item.metadata.get",
		Params:             reflect.TypeOf(PathRequest{}),
		Result:             reflect.TypeOf(FileMetadata{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    fileMetadataGetAPI,
	}
	endpoints[fileMetaGet.Method] = fileMetaGet

	fileMetaEdit := internal.Catalog{
		Name:               "File Metadata Change",
		Description:        "Retrieves file metadata header",
		Method:             "fs.item.metadata.edit",
		Params:             reflect.TypeOf(FileMetadata{}),
		Result:             reflect.TypeOf(NilSuccess{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    fileMetadataEditAPI,
	}
	endpoints[fileMetaEdit.Method] = fileMetaEdit

	// ====================== FILESYSTEM OPERATIONS ======================
	fsNew := internal.Catalog{
		Name:               "Create Item",
		Description:        "Creates empty file or directory",
		Method:             "fs.item.new",
		Params:             reflect.TypeOf(FileOp{}),
		Result:             reflect.TypeOf(NilSuccess{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    fsNewAPI,
	}
	endpoints[fsNew.Method] = fsNew

	fsDelete := internal.Catalog{
		Name:               "Delete Item",
		Description:        "Deletes file or directory",
		Method:             "fs.item.delete",
		Params:             reflect.TypeOf(FileOp{}),
		Result:             reflect.TypeOf(NilSuccess{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    fsDeleteAPI,
	}
	endpoints[fsDelete.Method] = fsDelete

	fsMove := internal.Catalog{
		Name:               "Move Item",
		Description:        "Moves/Copies file or directory",
		Method:             "fs.item.move",
		Params:             reflect.TypeOf(FileMove{}),
		Result:             reflect.TypeOf(NilSuccess{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    fsMoveAPI,
	}
	endpoints[fsMove.Method] = fsMove

	// ====================== FILESYSTEM SEARCH ======================
	fsPathSearch := internal.Catalog{
		Name:               "Path Search",
		Description:        "Searches for specific file or directory names",
		Method:             "fs.item.search",
		Params:             reflect.TypeOf(FilePathSearchReq{}),
		Result:             reflect.TypeOf(FilePathSearchResults{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    fsPathSearch,
	}
	endpoints[fsPathSearch.Method] = fsPathSearch

	// ====================== REPOSITORY ======================
	repoStatus := internal.Catalog{
		Name:               "Repository Status",
		Description:        "Retrieves current repository staging area status",
		Method:             "repo.staging.status",
		Params:             nil,
		Result:             reflect.TypeOf(RepoStatus{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    repoStatusAPI,
	}
	endpoints[repoStatus.Method] = repoStatus

	repoRefreshArtifact := internal.Catalog{
		Name:               "Refresh Artifacts",
		Description:        "Triggers tracking for remote artifact files",
		Method:             "repo.artifacts.refresh",
		Params:             nil,
		Result:             reflect.TypeOf(RepoStatus{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    repoRefreshAPI,
	}
	endpoints[repoRefreshArtifact.Method] = repoRefreshArtifact

	repoStageAdd := internal.Catalog{
		Name:               "Staging Add",
		Description:        "Add items from repository staging area",
		Method:             "repo.staging.add",
		Params:             reflect.TypeOf(PathList{}),
		Result:             reflect.TypeOf(RepoStatus{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    repoStageAddAPI,
	}
	endpoints[repoStageAdd.Method] = repoStageAdd

	repoStageDel := internal.Catalog{
		Name:               "Staging Remove",
		Description:        "Remove items rom the repository staging area",
		Method:             "repo.staging.remove",
		Params:             reflect.TypeOf(PathList{}),
		Result:             reflect.TypeOf(RepoStatus{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    repoStageRemoveAPI,
	}
	endpoints[repoStageDel.Method] = repoStageDel

	repoCommit := internal.Catalog{
		Name:               "Commit Changes",
		Description:        "Commit staged changes to repository",
		Method:             "repo.commit",
		Params:             reflect.TypeOf(RepoCommit{}),
		Result:             reflect.TypeOf(RepoCommitInfo{}),
		AllowedPermissions: []internal.PermissionAction{internal.WriteAction},
		HandlerFunction:    repoCommitAPI,
	}
	endpoints[repoCommit.Method] = repoCommit

	repoDiff := internal.Catalog{
		Name:               "File Difference",
		Description:        "Get the difference in file content between two commits",
		Method:             "repo.commit.diff",
		Params:             reflect.TypeOf(RepoFileDiffReq{}),
		Result:             reflect.TypeOf(RepoFileDiffResp{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    repoDiffAPI,
	}
	endpoints[repoDiff.Method] = repoDiff

	repoHistory := internal.Catalog{
		Name:               "Repository History",
		Description:        "Get commit history for the repository",
		Method:             "repo.commit.history",
		Params:             reflect.TypeOf(PaginationReq{}),
		Result:             reflect.TypeOf([]RepoCommitInfo{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    repoHistoryAPI,
	}
	endpoints[repoHistory.Method] = repoHistory

	// ====================== DEPLOYMENTS ======================
	startDeploy := internal.Catalog{
		Name:               "Start Deployment",
		Description:        "Trigger new deployment with provided options",
		Method:             "deployment.start",
		Params:             reflect.TypeOf(DeployStart{}),
		Result:             reflect.TypeOf(DeployStatus{}),
		AllowedPermissions: []internal.PermissionAction{internal.DeployAction},
		HandlerFunction:    deploymentNewAPI,
	}
	endpoints[startDeploy.Method] = startDeploy

	deployAbort := internal.Catalog{
		Name:               "Abort Deployment",
		Description:        "Cancel a specific deployment",
		Method:             "deployment.abort",
		Params:             reflect.TypeOf(DeployAbort{}),
		Result:             reflect.TypeOf(DeployStatus{}),
		AllowedPermissions: []internal.PermissionAction{internal.DeployAction},
		HandlerFunction:    deploymentAbortAPI,
	}
	endpoints[deployAbort.Method] = deployAbort

	deployStatus := internal.Catalog{
		Name:               "Check Deployment",
		Description:        "Retrieve specific deployment status",
		Method:             "deployment.status",
		Params:             reflect.TypeOf(DeployStatusReq{}),
		Result:             reflect.TypeOf(DeployStatus{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    deploymentStatusAPI,
	}
	endpoints[deployStatus.Method] = deployStatus

	deployOutput := internal.Catalog{
		Name:               "Deployment Result",
		Description:        "Retrieve deployment summary and action report",
		Method:             "deployment.result",
		Params:             reflect.TypeOf(DeployStatusReq{}),
		Result:             reflect.TypeOf(DeployOutput{}),
		AllowedPermissions: []internal.PermissionAction{internal.ReadAction},
		HandlerFunction:    deploymentOutputAPI,
	}
	endpoints[deployOutput.Method] = deployOutput

	// ====================== SEED ======================

	seedRequest := internal.Catalog{
		Name:               "Seed Request",
		Description:        "Initiate repository seed from remote hosts",
		Method:             "seed.start",
		Params:             reflect.TypeOf(SeedRequest{}),
		Result:             reflect.TypeOf(SeedResp{}),
		AllowedPermissions: []internal.PermissionAction{internal.SeedAction},
		HandlerFunction:    seedAPI,
	}
	endpoints[seedRequest.Method] = seedRequest

	seedSelection := internal.Catalog{
		Name:               "Seed Selection",
		Description:        "Submit selections to download to repository",
		Method:             "seed.selection.get",
		Params:             reflect.TypeOf(SeedSelections{}),
		Result:             reflect.TypeOf(SeedSelectSuccess{}),
		AllowedPermissions: []internal.PermissionAction{internal.SeedAction},
		HandlerFunction:    seedSelectionsAPI,
	}
	endpoints[seedSelection.Method] = seedSelection

	return
}
