// controller
package main

import (
	"testing"
)

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
		deploymentList           DeploymentList
		totalDeployedReloadFiles map[string]int
		reloadIDreadyToReload    map[string]bool
		filePath                 string
		remoteModified           bool
		disableReloads           bool
		forceEnabled             bool
		expectedClearedToReload  bool
		expectedReloadID         string
	}{
		{
			name: "Remote modified, reloads enabled",
			deploymentList: DeploymentList{
				fileToReloadID: map[string]string{
					"file1": "reload1",
				},
				reloadIDfileCount: map[string]int{"reload1": 1},
			},
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDreadyToReload:    map[string]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           true,
			expectedClearedToReload:  true,
			expectedReloadID:         "reload1",
		},
		{
			name: "Remote modified, Group not done",
			deploymentList: DeploymentList{
				fileToReloadID: map[string]string{
					"file1": "reload1",
					"file2": "reload1",
				},
				reloadIDfileCount: map[string]int{"reload1": 2},
			},
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDreadyToReload:    map[string]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           true,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
		{
			name: "Previous Remote modified, Current Not Modified, Reload",
			deploymentList: DeploymentList{
				fileToReloadID: map[string]string{
					"file1": "reload1",
					"file2": "reload1",
				},
				reloadIDfileCount: map[string]int{"reload1": 2},
			},
			totalDeployedReloadFiles: map[string]int{"reload1": 1},
			reloadIDreadyToReload:    map[string]bool{"reload1": true},
			filePath:                 "file2",
			remoteModified:           false,
			expectedClearedToReload:  true,
			expectedReloadID:         "reload1",
		},
		{
			name: "No remote modification, reloads disabled",
			deploymentList: DeploymentList{
				fileToReloadID: map[string]string{
					"file1": "reload1",
				},
				reloadIDfileCount: map[string]int{"reload1": 1},
			},
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDreadyToReload:    map[string]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           false,
			disableReloads:           true,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
		{
			name: "Remote modification, reloads disabled",
			deploymentList: DeploymentList{
				fileToReloadID: map[string]string{
					"file1": "reload1",
				},
				reloadIDfileCount: map[string]int{"reload1": 1},
			},
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDreadyToReload:    map[string]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           true,
			disableReloads:           true,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
		{
			name: "Force enable reloads by user request",
			deploymentList: DeploymentList{
				fileToReloadID: map[string]string{
					"file1": "reload1",
				},
				reloadIDfileCount: map[string]int{"reload1": 1},
			},
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDreadyToReload:    map[string]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           false,
			forceEnabled:             true,
			expectedClearedToReload:  true,
			expectedReloadID:         "reload1",
		},
		{
			name: "No modification",
			deploymentList: DeploymentList{
				fileToReloadID: map[string]string{
					"file1": "reload1",
				},
				reloadIDfileCount: map[string]int{"reload1": 1},
			},
			totalDeployedReloadFiles: map[string]int{"reload1": 0},
			reloadIDreadyToReload:    map[string]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           false,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
	}

	// Iterate over each test case
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Override global config variables based on the test case
			config.options.disableReloads = test.disableReloads
			config.options.forceEnabled = test.forceEnabled

			clearedToReload, reloadID := checkForReload("", test.deploymentList, test.totalDeployedReloadFiles, test.reloadIDreadyToReload, test.filePath, test.remoteModified)

			// Check if the result matches the expected outcome
			if reloadID != test.expectedReloadID {
				t.Errorf("Expected Reload ID = %s, but got '%s'", test.expectedReloadID, reloadID)
			}
			if clearedToReload != test.expectedClearedToReload {
				t.Errorf("Expected clearedToReload = %v, but got '%v'", test.expectedClearedToReload, clearedToReload)
			}
		})
	}
}
