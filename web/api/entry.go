package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"scmp/internal/config"
	"scmp/internal/config/sshconfig"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/web/internal"
)

// Process all requests to the /api path
func HandleAPI(baseCtx context.Context, serverResponder http.ResponseWriter, clientRequest *http.Request) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			respondWithError(serverResponder, "", rpcInternalError, "Critical failure", "check server logs")
			logctx.LogEvent(baseCtx, logctx.VerbosityStandard, logctx.FatalLog, "Encountered critical error while processing client API request: %v\n", fatalError)
		}
	}()

	// Extract actual request from HTTP
	api, req, err := parseUserRequest(serverResponder, clientRequest)
	if err != nil {
		return
	}

	// Set user settings
	clientCtx, err := setUserConfig(clientRequest, api)
	if err != nil {
		respondWithError(serverResponder, req.ID, rpcInternalError, "failed to marshal result", err.Error())
		return
	}

	// Evaluate user permissions
	authorized, err := checkUserPermissions(clientCtx, api)
	if err != nil {
		respondWithError(serverResponder, req.ID, rpcInternalError, "failed to marshal result", err.Error())
		return
	}
	if !authorized {
		respondWithError(serverResponder, req.ID, rpcUnauthorized, "user does not have sufficient permissions for method", "")
		return
	}

	// Call the handler function
	result, errObj := api.HandlerFunction(baseCtx, clientCtx, req)

	resp := internal.Response{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	// Evaluate handler response
	if errObj.Message != "" {
		resp.Error = &errObj
	} else if result != nil {
		// Add generic success message to APIs that don't return anything
		nilResult, nilButSucceeded := result.(NilSuccess)
		if nilButSucceeded {
			if nilResult.Status == "" {
				nilResult.Status = "succeeded"
			}
			result = nilResult
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			respondWithError(serverResponder, req.ID, rpcInternalError, "failed to marshal result", err.Error())
			return
		} else {
			resp.Result = resultJSON
		}
	}

	// Send response back to client
	serverResponder.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(serverResponder)
	_ = encoder.Encode(resp)
}

func parseUserRequest(serverResponder http.ResponseWriter, clientRequest *http.Request) (api internal.Catalog, req internal.Request, err error) {
	// Validate HTTP
	if clientRequest.Method != http.MethodPost {
		respondWithError(serverResponder, "", rpcInvalidRequest, "Invalid transport protocol parameter", "only HTTP post methods are supported")
		return
	}

	// Decode raw JSON-RPC request
	var rawReq struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
		ID      string          `json:"id"`
	}
	err = json.NewDecoder(clientRequest.Body).Decode(&rawReq)
	if err != nil {
		respondWithError(serverResponder, "", rpcParseErr, "Parse error", err.Error())
		return
	}

	// Lookup the API method
	api, validMethod := internal.GetAPIDef()[rawReq.Method]
	if !validMethod {
		respondWithError(serverResponder, rawReq.ID, rpcMethodNotFound, "Method not found: "+rawReq.Method, "")
		return
	}

	// Unmarshal params into the correct struct
	var param interface{}
	if api.Params != nil {
		param = reflect.New(api.Params).Interface()

		err = json.Unmarshal(rawReq.Params, param)
		if err != nil {
			respondWithError(serverResponder, rawReq.ID, rpcInvalidParams, "Invalid params", err.Error())
			return
		}

		// Dereference pointer to struct
		param = reflect.ValueOf(param).Elem().Interface()
	}

	// Construct typed Request
	req = internal.Request{
		JSONRPC: rawReq.JSONRPC,
		Method:  rawReq.Method,
		Params:  param,
		ID:      rawReq.ID,
	}

	return
}

func setUserConfig(clientRequest *http.Request, api internal.Catalog) (clientCtx context.Context, err error) {
	clientCtx = context.Background()

	// Return immediately for unauthenticated users
	if api.Method == "user.login" {
		return
	}

	// Copy values from request (avoid inheriting cancellations)
	httpCtx := clientRequest.Context()

	// Retrieve username (set in auth middleware)
	username := global.AssertFromContext[string](httpCtx, "username", global.UserKey, "string")
	clientCtx = context.WithValue(clientCtx, global.UserKey, username)

	email := global.AssertFromContext[string](httpCtx, "username", global.UserKey, "string")
	clientCtx = context.WithValue(clientCtx, global.EmailKey, email)

	// User name should always be present
	if username == "" {
		err = fmt.Errorf("no username in request")
		return
	}

	// Retrieve permissions for this user to initialize configurations
	userPermissions := global.AssertFromContext[[]internal.UserPermission](httpCtx, "userPermissions", global.PermKey, "[]internal.UserPermission")
	clientCtx = context.WithValue(clientCtx, global.PermKey, userPermissions)

	// Extract repository info from header (client must send it)
	selectedRepo := clientRequest.Header.Get(internal.RepoHeaderName)
	if selectedRepo == "" {
		// Optionally fall back to user’s default repo
		if len(userPermissions) > 0 {
			selectedRepo = userPermissions[0].Repo
		}
	}

	repoConf := internal.GetRepoConfig()

	// Set repo path based on web config (not using current directory)
	var emptyConfig config.Config
	emptyConfig.RepositoryPath = repoConf[selectedRepo].RootPath
	clientCtx = context.WithValue(clientCtx, global.ConfKey, emptyConfig)

	// Set configuration for this user
	clientCtx, err = sshconfig.Set(clientCtx, repoConf[selectedRepo].SSHConfigPath)
	if err != nil {
		err = fmt.Errorf("failed loading user '%s' configuration: %w", username, err)
		return
	}
	return
}
