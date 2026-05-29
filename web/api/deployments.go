package api

import (
	"context"
	"encoding/json"
	"fmt"
	"scmp/cli"
	"scmp/core/deployment/local"
	"scmp/core/deployment/metrics"
	"scmp/internal/config"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/web/api/prompt"
	"scmp/web/datastore"
	"scmp/web/internal"
	"strings"

	"github.com/google/uuid"
)

func deploymentNewAPI(ctx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[DeployStart](fullReq.Params, "req", "DeployStart")

	// Check mode against CLI subcommands for validity
	if !cli.IsValidSubcommand(cli.GetCLICmds(), "deploy", req.Mode) {
		errObj.New(rpcInvalidParams, "Invalid Request", fmt.Sprintf("mode %s is not a valid mode", req.Mode))
		return
	}

	var opts config.Opts

	switch strings.ToLower(req.Type) {
	case "preview":
		opts.DryRunEnabled = true
	case "test":
		opts.WetRunEnabled = true
	case "live":
		// Standard - deploy full
	default:
		errObj.New(rpcInvalidParams, "Invalid Request", fmt.Sprintf("type %s is not a valid type", req.Type))
		return
	}

	// Set options from request
	opts.AllowDeletions = req.Opts.AllowDeletions
	opts.RunInstallCommands = req.Opts.RunInstallCmds
	opts.DisableReloads = req.Opts.DisableReloads
	opts.DisableSudo = req.Opts.DisableSudo
	opts.IgnoreDeploymentState = req.Opts.IgnoreHostState
	opts.ForceEnabled = req.Opts.Force
	if req.Opts.MaxSSHConn != 0 {
		opts.MaxSSHConcurrency = req.Opts.MaxSSHConn
	} else {
		opts.MaxSSHConcurrency = sshinternal.MaxSSHConnections
	}
	if req.Opts.MaxSSHChannels != 0 {
		opts.MaxDeployConcurrency = req.Opts.MaxSSHChannels
	} else {
		opts.MaxDeployConcurrency = sshinternal.MaxSSHChannels
	}
	if req.Opts.CommandTimeout != 0 {
		opts.ExecutionTimeout = req.Opts.CommandTimeout
	} else {
		opts.ExecutionTimeout = sshinternal.DefaultCommandTimeout
	}
	if req.Opts.RunAsUser != "" {
		opts.RunAsUser = req.Opts.RunAsUser
	}
	// Always request full
	opts.DetailedSummaryRequested = true

	// Add deployment options to client context
	clientCtx, abortFunc := context.WithCancel(clientCtx)
	clientCtx = context.WithValue(clientCtx, global.OpsKey, opts)

	username := global.AssertFromContext[string](clientCtx, "username", global.UserKey, "string")

	deploymentID := uuid.New()
	deploymentStatus := "started"
	resp = DeployStatus{
		ID:     deploymentID.String(),
		Status: deploymentStatus,
	}

	// Set identifier for prompting
	promptID := uuid.New().String()
	clientCtx = context.WithValue(clientCtx, global.IDKey, promptID) // Setting in context for retrieval by prompt functions

	tracker := deploymentTracker{
		status:           deploymentStatus,
		associatedDataID: promptID,
		abort:            abortFunc,
	}
	datastore.Put(username, deploymentID.String(), tracker)

	// Setup logger scoped to user and this deployment
	contextID := fmt.Sprintf("%s+%s", username, deploymentID)

	clientCtx = logctx.New(clientCtx, contextID, req.Opts.Verbosity, clientCtx.Done())
	logger := logctx.GetLogger(clientCtx)

	go func() {
		var deployErr string

		tracker.status = "running"
		datastore.Put(username, deploymentID.String(), tracker)

		rollbackCommit, err := local.StartDeploy(clientCtx, req.Mode, req.Opts.CommitID, req.Opts.HostOverride, req.Opts.FileOverride)

		tracker.status = "parsing output"
		datastore.Put(username, deploymentID.String(), tracker)

		// Deployment error
		if err != nil {
			deployErr += fmt.Sprintf("Deployment Failed: %v\n", err)
			err = gitinternal.RollBackOneCommit(ctx, req.Opts.CommitID, req.Opts.AutoCommitRollback, rollbackCommit)
			if err != nil {
				deployErr += fmt.Sprintf("Error rolling back commit. %v\n", err)
			}
		}

		// Deployment progress messages
		if deployErr != "" {
			tracker.errorStr = deployErr
			datastore.Put(username, deploymentID.String(), tracker)
		} else {
			tracker.output = logger.GetFormattedLogLines()
			datastore.Put(username, deploymentID.String(), tracker)
		}

		tracker.status = "finished"
		datastore.Put(username, deploymentID.String(), tracker)
	}()
	return
}

func deploymentAbortAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[DeployAbort](fullReq.Params, "req", "DeployAbort")
	if !req.StopRequested {
		return
	}

	username := global.AssertFromContext[string](clientCtx, "username", global.UserKey, "string")

	// Retrieve abort from datastore
	data, err := datastore.Get(username, req.ID)
	if err != nil {
		errObj.New(rpcInvalidParams, "Failed to retrieve state from datastore", err.Error())
		return
	}
	tracker := global.AssertType[deploymentTracker](data, "tracker", "deploymentTracker")

	// Send cancellation
	tracker.abort()

	// Update status with request
	tracker.status = "cancellation requested"
	datastore.Put(username, req.ID, tracker)

	resp = DeployStatus{
		ID:     req.ID,
		Status: tracker.status,
	}
	return
}

func deploymentStatusAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[DeployStatusReq](fullReq.Params, "req", "DeployStatusReq")

	username := global.AssertFromContext[string](clientCtx, "username", global.UserKey, "string")

	data, err := datastore.Get(username, req.ID)
	if err != nil {
		errObj.New(rpcInvalidParams, "Failed to retrieve state from datastore", err.Error())
		return
	}
	tracker := global.AssertType[deploymentTracker](data, "tracker", "deploymentTracker")

	respStatus := DeployStatus{
		ID:     req.ID,
		Status: tracker.status,
	}

	// Check for pending prompts
	if prompt.HasPending(username, tracker.associatedDataID) {
		respStatus.Pending = true

		prompts, err := prompt.GetPending(username, tracker.associatedDataID)
		if err != nil {
			errObj.New(rpcInternalError, "Failed to retrieve pending prompts", "")
			return
		}

		respStatus.PendingAction = prompts
	}

	resp = respStatus
	return
}

func deploymentOutputAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[DeployStatusReq](fullReq.Params, "req", "DeployStatusReq")

	username := global.AssertFromContext[string](clientCtx, "username", global.UserKey, "string")

	data, err := datastore.Get(username, req.ID)
	if err != nil {
		errObj.New(rpcInvalidParams, "Failed to retrieve state from datastore", err.Error())
		return
	}

	tracker, ok := data.(deploymentTracker)
	if !ok {
		errObj.New(rpcInternalError, "Failed type assertion datastore tracker", "")
		return
	}

	if tracker.status != "finished" {
		errObj.New(rpcInvalidParams, "Deployment ID "+req.ID+" is not finished and is in state %s"+tracker.status, "")
		return
	}

	// Response object
	var formattedOutput DeployOutput
	formattedOutput.ID = req.ID
	formattedOutput.Status = tracker.status

	// Get summary JSON
	if len(tracker.output) > 0 {
		lastLog := tracker.output[len(tracker.output)-1]
		if strings.HasPrefix(strings.TrimSpace(lastLog), "{") {
			var summary metrics.Summary
			err := json.Unmarshal([]byte(lastLog), &summary)
			if err != nil {
				errObj.New(rpcInvalidParams, "Failed to parse summary JSON", err.Error())
				return
			}
			formattedOutput.Details = summary
		}
	}

	if tracker.errorStr != "" {
		tracker.output = append(tracker.output, tracker.errorStr)
		formattedOutput.Details.Status = "Failed"
	}

	// Convert raw events to string
	fullLog := strings.Join(tracker.output, " ")
	formattedOutput.RawOutput = fullLog

	resp = formattedOutput

	// Cleanup buffer
	datastore.Delete(username, req.ID)
	return
}
