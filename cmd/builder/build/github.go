package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"strconv"
	"strings"
)

type createReleaseReq struct {
	Tag               string `json:"tag_name"`
	TgtBranch         string `json:"target_commitish"`
	Name              string `json:"name"` // Empty to use tag
	Body              string `json:"body"`
	Draft             bool   `json:"draft"`
	PreRelease        bool   `json:"prerelease"`
	AutoGenerateNotes bool   `json:"generate_release_notes"` // always false
}

type createReleaseResp struct {
	ID      *int64 `json:"id"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url"` // API url
	HTMLURL string `json:"html_url"`
}

type uploadAssetResp struct {
	State string `json:"state"`
}

func publishRelease(ctx *context) (err error) {
	localReleaseDir, err := createReleaseStagingDir()
	if err != nil {
		err = fmt.Errorf("release staging: %w", err)
		return
	}
	releaseChangeLogFile := filepath.Join(localReleaseDir, "release-notes.md")

	githubAPIToken := os.Getenv("GITHUB_API_TOKEN")
	if githubAPIToken == "" {
		err = fmt.Errorf("GITHUB_API_TOKEN environment variable is not set, cannot release")
		return
	}

	printInfo(0, "Creating new Github release with notes from file %s", releaseChangeLogFile)

	changeLog, err := os.ReadFile(releaseChangeLogFile)
	if err != nil {
		err = fmt.Errorf("failed to read change log file: %w", err)
		return
	}

	constsFilePath := filepath.Join(ctx.repositoryRoot, globalConstsFile)
	constsFile, err := os.ReadFile(constsFilePath)
	if err != nil {
		err = fmt.Errorf("failed to read main program constants file for version: %w", err)
		return
	}
	progVersion, err := helpers.GetProgVersion(constsFile, versionVariableName)
	if err != nil {
		err = fmt.Errorf("version retrieval: %w", err)
		return
	}

	// Pre-release if major version number is 0
	var preRelease bool
	if strings.HasPrefix(progVersion, "v0.") {
		preRelease = true
	}

	release := createReleaseReq{
		Tag:               progVersion,
		TgtBranch:         "main",
		Name:              "",
		Body:              string(changeLog),
		Draft:             false,
		PreRelease:        preRelease,
		AutoGenerateNotes: false,
	}

	releaseJSON, err := json.Marshal(release)
	if err != nil {
		err = fmt.Errorf("invalid release JSON: %w", err)
		return
	}
	releaseBodyReader := bytes.NewReader(releaseJSON)

	createURL := "https://" + baseAPI + "/repos/" + ctx.cfg.RemoteGitUsername + "/" + ctx.cfg.RemoteGitRepo + "/releases"
	parsedURL, err := url.Parse(createURL)
	if err != nil {
		err = fmt.Errorf("invalid release create endpoint URL: %w", err)
		return
	}

	releaseHTTPReq, err := http.NewRequest("POST", parsedURL.String(), releaseBodyReader)
	if err != nil {
		err = fmt.Errorf("failed to create release HTTP request: %w", err)
		return
	}
	releaseHTTPReq.Header.Add("Accept", "application/vnd.github+json")
	releaseHTTPReq.Header.Add("Authorization", "Bearer "+githubAPIToken)
	releaseHTTPReq.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	releaseHTTPResp, err := http.DefaultClient.Do(releaseHTTPReq)
	if err != nil {
		err = fmt.Errorf("failed to send release request to github: %w", err)
		return
	}
	wholeBody, err := helpers.HTTPCheckResp(releaseHTTPResp)
	if err != nil {
		err = fmt.Errorf("create release: %w", err)
		return
	}

	var releaseResp createReleaseResp
	err = json.Unmarshal(wholeBody, &releaseResp)
	if err != nil {
		err = fmt.Errorf("failed to parse release response JSON body: %w", err)
		return
	}

	if releaseResp.ID == nil || *releaseResp.ID == 0 {
		err = fmt.Errorf("unable to extract release ID from release create response: (%s) %s",
			releaseResp.Status, releaseResp.Message)
		return
	}
	releaseID := *releaseResp.ID

	finalReleaseURL := releaseResp.HTMLURL

	printSuccess(0, "Successfully created new Github release - ID: %d", releaseID)

	// Release is created, change log is not required (will be treating all staging contents as file attachments now)
	_ = os.Remove(releaseChangeLogFile)

	stagingItems, err := os.ReadDir(localReleaseDir)
	if err != nil {
		err = fmt.Errorf("failed to list release staging directory entries: %w", err)
		return
	}

	baseAttachmentUploadURL := "https://" + uploadAPI + "/repos/" + ctx.cfg.RemoteGitUsername + "/" + ctx.cfg.RemoteGitRepo + "/releases/" + strconv.Itoa(int(releaseID)) + "/assets"
	parsedUploadURL, err := url.Parse(baseAttachmentUploadURL)
	if err != nil {
		err = fmt.Errorf("invalid release asset upload endpoint URL: %w", err)
		return
	}

	for _, stagingItem := range stagingItems {
		if stagingItem.IsDir() {
			continue
		}

		printInfo(2, "Uploading file %s to release %d", stagingItem.Name(), releaseID)

		path := filepath.Join(localReleaseDir, stagingItem.Name())
		var assetFile *os.File
		assetFile, err = os.OpenFile(path, os.O_RDONLY, 0600)
		if err != nil {
			err = fmt.Errorf("failed to read asset file: %w", err)
			return
		}

		queryParams := parsedUploadURL.Query()
		queryParams.Add("name", stagingItem.Name())
		parsedUploadURL.RawQuery = queryParams.Encode()

		var uploadRequest *http.Request
		uploadRequest, err = http.NewRequest("POST", parsedUploadURL.String(), assetFile)
		if err != nil {
			err = fmt.Errorf("failed to create release HTTP request: %w", err)
			return
		}
		uploadRequest.Header.Add("Accept", "application/vnd.github+json")
		uploadRequest.Header.Add("Authorization", "Bearer "+githubAPIToken)
		uploadRequest.Header.Add("X-GitHub-Api-Version", "2022-11-28")
		uploadRequest.Header.Add("Content-Type", "application/octet-stream")

		var uploadRawResp *http.Response
		uploadRawResp, err = http.DefaultClient.Do(uploadRequest)
		if err != nil {
			err = fmt.Errorf("failed to send upload request to github: %w", err)
			return
		}

		var wholeBody []byte
		wholeBody, err = helpers.HTTPCheckResp(uploadRawResp)
		if err != nil {
			err = fmt.Errorf("asset %s: %w", stagingItem.Name(), err)
			return
		}

		var uploadResp uploadAssetResp
		err = json.Unmarshal(wholeBody, &uploadResp)
		if err != nil {
			err = fmt.Errorf("failed to parse release upload response JSON body: %w", err)
			return
		}

		if uploadResp.State != "uploaded" {
			err = fmt.Errorf("asset %s: expected state to be uploaded but got %s from github", stagingItem.Name(), uploadResp.State)
			return
		}

		printSuccess(2, "Successfully uploaded file %s to release %d", stagingItem.Name(), releaseID)
	}

	printSuccess(0, "Release published: %s", finalReleaseURL)

	// Cleanup
	err = os.RemoveAll(localReleaseDir)
	if err != nil {
		printWarn(0, "Failed to remove release staging: %w", err)
		err = nil
	}
	return
}
