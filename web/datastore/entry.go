// Package for generic data upload and download for use with the web interface
package datastore

import (
	"encoding/json"
	"io"
	"net/http"
	"scmp/internal/global"
	"scmp/web/internal"
	"strings"

	"github.com/google/uuid"
)

func HandleBytes(serverResponder http.ResponseWriter, clientRequest *http.Request) {
	path := clientRequest.URL.Path
	ctx := clientRequest.Context()
	userID := global.AssertFromContext[string](ctx, "userID", global.UserKey, "string")

	if path == internal.UploadPath {
		if clientRequest.Method != http.MethodPost {
			http.Error(serverResponder, "Invalid Method", http.StatusMethodNotAllowed)
			return
		}

		data, err := io.ReadAll(clientRequest.Body)
		if err != nil {
			http.Error(serverResponder, "Failed to read request data", http.StatusBadRequest)
			return
		}

		newDataID := uuid.New()

		Put(userID, newDataID.String(), data)

		resp := internal.Response{
			JSONRPC: "2.0",
			ID:      "1",
			Result:  json.RawMessage(`"` + newDataID.String() + `"`),
		}

		serverResponder.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(serverResponder)
		_ = encoder.Encode(resp)
	} else if strings.HasPrefix(path, internal.DownloadBasePath) {
		if clientRequest.Method != http.MethodGet {
			http.Error(serverResponder, "Invalid Method", http.StatusMethodNotAllowed)
			return
		}

		lastPathItem := strings.LastIndex(path, "/")
		if lastPathItem == -1 || lastPathItem == len(path)-1 {
			serverResponder.WriteHeader(http.StatusNotFound)
			return
		}
		dataID := path[lastPathItem+1:]

		_, err := uuid.Parse(dataID)
		if err != nil {
			http.Error(serverResponder, "Invalid Data ID: "+err.Error(), http.StatusBadRequest)
			return
		}

		data, err := Get(userID, dataID)
		if err != nil {
			http.Error(serverResponder, "Unable to retrieve data: "+err.Error(), http.StatusBadRequest)
			return
		}

		bytes, ok := data.([]byte)
		if !ok {
			http.Error(serverResponder, "Unable to retrieve data: requested ID is not raw bytes", http.StatusInternalServerError)
			return
		}

		serverResponder.Header().Set("Content-Type", "application/octet-stream")
		_, _ = serverResponder.Write(bytes)

		// Delete data after sending
		Delete(userID, dataID)
	} else {
		serverResponder.WriteHeader(http.StatusNotFound)
		return
	}
}
