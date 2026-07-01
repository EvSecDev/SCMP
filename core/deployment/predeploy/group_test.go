package predeploy

import (
	"scmp/core/deployment"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"slices"
	"testing"
)

func TestRemoveRedundantDeletions(t *testing.T) {
	// Mock Global
	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())

	tests := []struct {
		name               string
		files              map[str.LocalRepoPath]deployment.FileInfo
		expectedPathRemove str.LocalRepoPath
	}{
		{
			name: "Host file deleted, same file added in universal group",
			files: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/file1.txt": {
					Hash:           "abc",
					TargetFilePath: "/etc/file1.txt",
					Action:         deployment.ActionDelete,
				},
				"UniversalConfs_Service1/etc/file1.txt": {
					Hash:           "def",
					TargetFilePath: "/etc/file1.txt",
					Action:         deployment.ActionCreate,
				},
			},
			expectedPathRemove: "host1/etc/file1.txt",
		},
		{
			name: "Universal file deleted, same file added in host",
			files: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/file1.txt": {
					Hash:           "abc",
					TargetFilePath: "/etc/file1.txt",
					Action:         deployment.ActionCreate,
				},
				"UniversalConfs_Service1/etc/file1.txt": {
					Hash:           "def",
					TargetFilePath: "/etc/file1.txt",
					Action:         deployment.ActionDelete,
				},
			},
			expectedPathRemove: "UniversalConfs_Service1/etc/file1.txt",
		},
		{
			name: "Only created file",
			files: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/file1.txt": {
					Hash:           "abc",
					TargetFilePath: "/etc/file1.txt",
					Action:         deployment.ActionCreate,
				},
				"UniversalConfs_Service1/etc/file21.txt": {
					Hash:           "def",
					TargetFilePath: "/etc/file2.txt",
					Action:         deployment.ActionCreate,
				},
			},
		},
		{
			name: "One deleted file",
			files: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/file1.txt": {
					Hash:           "abc",
					TargetFilePath: "/etc/file1.txt",
					Action:         deployment.ActionDelete,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hostFiles, err := deployment.NewHostFiles()
			if err != nil {
				t.Fatalf("unexpected hostfiles create failure: %v", err)
			}

			var flatFileList []str.LocalRepoPath
			for repoFilePath, fileInfo := range test.files {
				hostFiles.SetFileMetadata(repoFilePath, fileInfo)
				hostFiles.StoreDataOnce(fileInfo.Hash, []byte("placeholder content"))
				flatFileList = append(flatFileList, repoFilePath)
			}
			slices.Sort(flatFileList)

			newGroup := deployment.NewFileGroup(flatFileList)
			hostFiles.Groups = append(hostFiles.Groups, newGroup)

			err = removeRedundantDeletions(ctx, hostFiles)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			notSeenFiles := make(map[str.LocalRepoPath]bool)
			for _, group := range hostFiles.Groups {
				if test.expectedPathRemove != "" && slices.Contains(group.GetOrderedList(), test.expectedPathRemove) {
					t.Errorf("found path '%s' in hostFiles group deploy list: %v", test.expectedPathRemove, group.GetOrderedList())
				}

				for path := range test.files {
					if path == test.expectedPathRemove {
						continue
					}

					notSeenFiles[path] = true
					if slices.Contains(group.GetOrderedList(), path) {
						delete(notSeenFiles, path)
					}
				}
			}
			if len(notSeenFiles) > 0 {
				t.Errorf("original file list missing files post test run: %#v", notSeenFiles)
			}
		})
	}
}
