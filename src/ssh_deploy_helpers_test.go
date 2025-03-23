// controller
package main

import (
	"testing"
)

// CompareMaps compares two maps of type map[string][]string
func compareMaps(map1, map2 map[string][]string) bool {
	// First check if the lengths of the maps are equal
	if len(map1) != len(map2) {
		return false
	}

	// Check if all keys and their associated values are equal
	for key, val1 := range map1 {
		val2, ok := map2[key]
		if !ok {
			// Key doesn't exist in map2
			return false
		}

		// Check if the slices have the same length
		if len(val1) != len(val2) {
			return false
		}

		// Check if the slices have the same elements in the same order
		for i := range val1 {
			if val1[i] != val2[i] {
				return false
			}
		}
	}

	// If we passed all checks, the maps are equal
	return true
}

// compareStringMapSlices compares two maps and checks if they are identical
func compareStringMapSlices(a, b map[string]string) bool {
	// If lengths are different, maps are not equal
	if len(a) != len(b) {
		return false
	}

	// Check if all key-value pairs are equal
	for key, value := range a {
		if bValue, exists := b[key]; !exists || bValue != value {
			return false
		}
	}
	return true
}

// compareStringSlices compares two slices and checks if they are identical
func compareStringSlices(a, b map[string]int) bool {
	// If lengths are different, they are not equal
	if len(a) != len(b) {
		return false
	}

	// Check if all key-value pairs are equal (we assume the "map" comparison)
	for key, value := range a {
		if bValue, exists := b[key]; !exists || bValue != value {
			return false
		}
	}

	return true
}

// Test function for groupFilesByReloads
func TestGroupFilesByReloads(t *testing.T) {
	tests := []struct {
		name               string
		allFileInfo        map[string]FileInfo
		repoFilePaths      []string
		reloadIDtoRepoFile map[string][]string
		repoFileToReloadID map[string]string
		reloadIDfileCount  map[string]int
	}{
		{
			name: "files with reload and no reload",
			allFileInfo: map[string]FileInfo{
				"file1": {reloadRequired: true, reload: []string{"cmd50", "cmd51", "cmd52"}},
				"file2": {reloadRequired: true, reload: []string{"cmd40", "cmd41"}},
				"file3": {reloadRequired: false, reload: nil},
			},
			repoFilePaths: []string{"file1", "file2", "file3"},
			reloadIDtoRepoFile: map[string][]string{
				"W2NtZDUwIGNtZDUxIGNtZDUyXQ==": {"file1"},
				"W2NtZDQwIGNtZDQxXQ==":         {"file2"},
			},
			repoFileToReloadID: map[string]string{
				"file1": "W2NtZDUwIGNtZDUxIGNtZDUyXQ==",
				"file2": "W2NtZDQwIGNtZDQxXQ==",
			},
			reloadIDfileCount: map[string]int{
				"W2NtZDUwIGNtZDUxIGNtZDUyXQ==": 1,
				"W2NtZDQwIGNtZDQxXQ==":         1,
			},
		},
		{
			name: "all files with the same reload command",
			allFileInfo: map[string]FileInfo{
				"file1": {reloadRequired: true, reload: []string{"cmd30", "cmd32", "cmd^$"}},
				"file2": {reloadRequired: true, reload: []string{"cmd30", "cmd32", "cmd^$"}},
				"file3": {reloadRequired: false, reload: nil},
			},
			repoFilePaths: []string{"file1", "file2", "file3"},
			reloadIDtoRepoFile: map[string][]string{
				"W2NtZDMwIGNtZDMyIGNtZF4kXQ==": {"file1", "file2"},
			},
			repoFileToReloadID: map[string]string{
				"file1": "W2NtZDMwIGNtZDMyIGNtZF4kXQ==",
				"file2": "W2NtZDMwIGNtZDMyIGNtZF4kXQ==",
			},
			reloadIDfileCount: map[string]int{
				"W2NtZDMwIGNtZDMyIGNtZF4kXQ==": 2,
			},
		},
		{
			name: "no files with reload commands",
			allFileInfo: map[string]FileInfo{
				"file1": {reloadRequired: false, reload: nil},
				"file2": {reloadRequired: false, reload: nil},
			},
			repoFilePaths:      []string{"file1", "file2"},
			reloadIDtoRepoFile: map[string][]string{},
			repoFileToReloadID: map[string]string{}, // No files with reloads
			reloadIDfileCount:  map[string]int{},
		},
		{
			name:               "empty input",
			allFileInfo:        map[string]FileInfo{},
			repoFilePaths:      []string{},
			reloadIDtoRepoFile: map[string][]string{},
			repoFileToReloadID: map[string]string{}, // No files
			reloadIDfileCount:  map[string]int{},    // No files
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Call the function being tested
			reloadIDtoRepoFile, repoFileToReloadID, reloadIDfileCount := groupFilesByReloads(test.allFileInfo, test.repoFilePaths)

			if !compareMaps(reloadIDtoRepoFile, test.reloadIDtoRepoFile) {
				t.Errorf("expected repoFileToReloadID: %v, got: %v", test.reloadIDtoRepoFile, reloadIDtoRepoFile)
			}

			if !compareStringMapSlices(repoFileToReloadID, test.repoFileToReloadID) {
				t.Errorf("expected repoFileToReloadID: %v, got: %v", test.repoFileToReloadID, repoFileToReloadID)
			}

			if !compareStringSlices(reloadIDfileCount, test.reloadIDfileCount) {
				t.Errorf("expected reloadIDfileCount: %v, got: %v", test.reloadIDfileCount, reloadIDfileCount)
			}

		})
	}
}

func TestCheckForDiff(t *testing.T) {
	tests := []struct {
		name                    string
		remoteMetadata          RemoteFileInfo
		localMetadata           FileInfo
		expectedContentDiffers  bool
		expectedMetadataDiffers bool
	}{
		{
			name: "Everything differs",
			remoteMetadata: RemoteFileInfo{
				hash:        "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
				permissions: 757,
				owner:       "user1",
				group:       "group1",
			},
			localMetadata: FileInfo{
				hash:        "590c9f8430c7435807df8ba9a476e3f1295d46ef210f6efae2043a4c085a569e",
				permissions: 640,
				ownerGroup:  "user2:group1",
			},
			expectedContentDiffers:  true,
			expectedMetadataDiffers: true,
		},
		{
			name: "Hashes differ",
			remoteMetadata: RemoteFileInfo{
				hash: "1b4f0e9851971998e732078544c96b36c3d01cedf7caa332359d6f1d83567014",
			},
			localMetadata: FileInfo{
				hash: "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
			},
			expectedContentDiffers:  true,
			expectedMetadataDiffers: false,
		},
		{
			name: "Permissions differ",
			remoteMetadata: RemoteFileInfo{
				permissions: 757,
			},
			localMetadata: FileInfo{
				permissions: 640,
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: true,
		},
		{
			name: "Owner and group differ",
			remoteMetadata: RemoteFileInfo{
				owner: "user1",
				group: "group1",
			},
			localMetadata: FileInfo{
				ownerGroup: "user2:group2",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: true,
		},
		{
			name: "No differences",
			remoteMetadata: RemoteFileInfo{
				hash:        "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
				permissions: 0755,
				owner:       "user1",
				group:       "group1",
			},
			localMetadata: FileInfo{
				hash:        "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
				permissions: 0755,
				ownerGroup:  "user1:group1",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: false,
		},
		{
			name: "No data",
			remoteMetadata: RemoteFileInfo{
				hash:        "",
				permissions: 0,
				owner:       "",
				group:       "",
			},
			localMetadata: FileInfo{
				hash:        "",
				permissions: 0,
				ownerGroup:  "",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contentDiffers, metadataDiffers := checkForDiff(test.remoteMetadata, test.localMetadata)

			if contentDiffers != test.expectedContentDiffers {
				t.Errorf("expected contentDiffers %v, got %v", test.expectedContentDiffers, contentDiffers)
			}
			if metadataDiffers != test.expectedMetadataDiffers {
				t.Errorf("expected metadataDiffers %v, got %v", test.expectedMetadataDiffers, metadataDiffers)
			}
		})
	}
}

func TestCheckForReload(t *testing.T) {
	// Define test cases
	tests := []struct {
		name                     string
		endpointName             string
		totalDeployedReloadFiles map[string]int
		reloadIDfileCount        map[string]int
		reloadID                 string
		remoteModified           bool
		disableReloads           bool
		forceEnabled             bool
		expectedClearedToReload  bool
	}{
		{
			name:                     "Remote modified, reloads enabled",
			endpointName:             "endpoint1",
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDfileCount:        map[string]int{"reload1": 1},
			reloadID:                 "reload1",
			remoteModified:           true,
			expectedClearedToReload:  true,
		},
		{
			name:                     "No remote modification, reloads disabled",
			endpointName:             "endpoint2",
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDfileCount:        map[string]int{"reload1": 1},
			reloadID:                 "reload1",
			remoteModified:           false,
			disableReloads:           true,
			expectedClearedToReload:  false,
		},
		{
			name:                     "Remote modification, reloads disabled",
			endpointName:             "endpoint2",
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDfileCount:        map[string]int{"reload1": 1},
			reloadID:                 "reload1",
			remoteModified:           true,
			disableReloads:           true,
			expectedClearedToReload:  false,
		},
		{
			name:                     "Force enable reloads by user request",
			endpointName:             "endpoint3",
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDfileCount:        map[string]int{"reload1": 1},
			reloadID:                 "reload1",
			remoteModified:           false,
			forceEnabled:             true,
			expectedClearedToReload:  true,
		},
		{
			name:                     "No modification",
			endpointName:             "endpoint4",
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDfileCount:        map[string]int{"reload1": 1},
			reloadID:                 "reload1",
			remoteModified:           false,
			expectedClearedToReload:  false,
		},
	}

	// Iterate over each test case
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Override global config variables based on the test case
			config.disableReloads = test.disableReloads
			config.forceEnabled = test.forceEnabled

			clearedToReload := checkForReload(test.endpointName, test.totalDeployedReloadFiles, test.reloadIDfileCount, test.reloadID, test.remoteModified)

			// Check if the result matches the expected outcome
			if clearedToReload != test.expectedClearedToReload {
				t.Errorf("For '%s', expected clearedToReload = %v, but got %v", test.name, test.expectedClearedToReload, clearedToReload)
			}
		})
	}
}
