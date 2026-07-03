package host

import (
	"context"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"testing"
)

func TestCheckForReload(t *testing.T) {
	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())

	// Define test cases
	tests := []struct {
		name                     string
		fileToReloadID           map[str.LocalRepoPath]str.ReloadID
		totalDeployedReloadFiles map[str.ReloadID]int
		reloadIDreadyToReload    map[str.ReloadID]bool
		filePath                 str.LocalRepoPath
		remoteModified           bool
		disableReloads           bool
		forceEnabled             bool
		expectedClearedToReload  bool
		expectedReloadID         str.ReloadID
	}{
		{
			name: "Remote modified, reloads enabled",
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file1": "reload1",
			},
			totalDeployedReloadFiles: map[str.ReloadID]int{"reload1": 0},
			reloadIDreadyToReload:    map[str.ReloadID]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           true,
			expectedClearedToReload:  true,
			expectedReloadID:         "reload1",
		},
		{
			name: "Remote modified, Group not done",
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file1": "reload1",
				"file2": "reload1",
			},
			totalDeployedReloadFiles: map[str.ReloadID]int{"reload1": 0},
			reloadIDreadyToReload:    map[str.ReloadID]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           true,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
		{
			name: "Previous Remote modified, Current Not Modified, Reload",
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file1": "reload1",
				"file2": "reload1",
			},
			totalDeployedReloadFiles: map[str.ReloadID]int{"reload1": 1},
			reloadIDreadyToReload:    map[str.ReloadID]bool{"reload1": true},
			filePath:                 "file2",
			remoteModified:           false,
			expectedClearedToReload:  true,
			expectedReloadID:         "reload1",
		},
		{
			name: "No remote modification, reloads disabled",
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file1": "reload1",
			},
			totalDeployedReloadFiles: map[str.ReloadID]int{"reload1": 0},
			reloadIDreadyToReload:    map[str.ReloadID]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           false,
			disableReloads:           true,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
		{
			name: "Remote modification, reloads disabled",
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file1": "reload1",
			},
			totalDeployedReloadFiles: map[str.ReloadID]int{"reload1": 0},
			reloadIDreadyToReload:    map[str.ReloadID]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           true,
			disableReloads:           true,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
		{
			name: "Force enable reloads by user request",
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file1": "reload1",
			},
			totalDeployedReloadFiles: map[str.ReloadID]int{"reload1": 0},
			reloadIDreadyToReload:    map[str.ReloadID]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           false,
			forceEnabled:             true,
			expectedClearedToReload:  true,
			expectedReloadID:         "reload1",
		},
		{
			name: "No modification",
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file1": "reload1",
			},
			totalDeployedReloadFiles: map[str.ReloadID]int{"reload1": 0},
			reloadIDreadyToReload:    map[str.ReloadID]bool{"reload1": false},
			filePath:                 "file1",
			remoteModified:           false,
			expectedClearedToReload:  false,
			expectedReloadID:         "",
		},
	}

	// Iterate over each test case
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var opts config.Opts
			opts.DisableReloads = test.disableReloads
			opts.ForceEnabled = test.forceEnabled
			ctx = context.WithValue(ctx, global.OpsKey, opts)

			mockFileGroup := deployment.NewFileGroup(nil)
			for file, reloadID := range test.fileToReloadID {
				mockFileGroup.AppendFileToReloadID(reloadID, file)
			}
			mockFileGroup.InitFiletoReloadID()
			mockFileGroup.RecordReloadIDFileCount()

			tracker := NewReloadTracker(mockFileGroup, &deployment.HostFiles{}, "testhost")
			tracker.totalDeployedReloadFiles = test.totalDeployedReloadFiles
			tracker.reloadIDreadyToReload = test.reloadIDreadyToReload

			clearedToReload, reloadID := tracker.CheckForReload(ctx, test.filePath, test.remoteModified)

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
