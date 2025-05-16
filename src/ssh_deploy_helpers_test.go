// controller
package main

import (
	"testing"
)

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
