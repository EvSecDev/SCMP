package predeploy

import (
	"context"
	"reflect"
	"scmp/core/deployment"
	"scmp/core/filesystem"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"slices"
	"testing"
)

func TestParseFileContent(t *testing.T) {
	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())

	config := config.Config{
		RepositoryPath: "/opt/repo",
		HostInfo: map[str.RepoRootDir]config.EndpointInfo{
			"host1": {},
		},
	}
	ctx = context.WithValue(ctx, global.ConfKey, config)

	type TestCase struct {
		name                string
		allDeploymentFiles  map[str.LocalRepoPath]str.DeployAction
		rawFileContent      map[str.LocalRepoPath][]byte
		expectedallFileMeta map[str.LocalRepoPath]deployment.FileInfo
		expectedallFileData map[str.FileID][]byte
		expectedErr         bool
	}
	testCases := []TestCase{
		{
			name: "Standard single input",
			allDeploymentFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/file1.conf": deployment.ActionFileCreate,
			},
			rawFileContent: map[str.LocalRepoPath][]byte{
				"host1/etc/file1.conf": []byte(`#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 644,
  "Dependencies": [
    "/etc/file2.conf"
  ],
  "Install": [
    "apt-get install pkg1 -y"
  ],
  "PostInstall": [
    "systemctl enable service1"
  ],
  "Preapply": [
    "ip a | grep ens18"
  ],
  "Postapply": [
    "ncat -nvz localhost:8080"
  ],
  "Reload": [
    "systemctl restart service1",
	"systemctl is-active service1"
  ]
}
#|^^^|#
some data here
more data here`),
			},
			expectedallFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/file1.conf": {
					Hash:              "72fd888f1aaeea80dd9d8da0082e2c2f6df9c796175b27066c2f71872547b8a9",
					RepoFilePath:      "host1/etc/file1.conf",
					TargetFilePath:    "/etc/file1.conf",
					Action:            deployment.ActionFileCreate,
					OwnerGroup:        "root:root",
					Permissions:       644,
					FileSize:          29,
					LinkTarget:        "",
					Dependencies:      []str.LocalRepoPath{"/etc/file2.conf"},
					InstallOptional:   true,
					Install:           []string{"apt-get install pkg1 -y"},
					PostInstall:       []string{"systemctl enable service1"},
					PreapplyRequired:  true,
					Preapply:          []string{"ip a | grep ens18"},
					PostapplyRequired: true,
					Postapply:         []string{"ncat -nvz localhost:8080"},
					ReloadRequired:    true,
					Reload:            []string{"systemctl restart service1", "systemctl is-active service1"},
				},
			},
			expectedallFileData: map[str.FileID][]byte{
				"72fd888f1aaeea80dd9d8da0082e2c2f6df9c796175b27066c2f71872547b8a9": []byte(`some data here
more data here`),
			},
			expectedErr: false,
		},
		{
			name: "Standard directory metadata input",
			allDeploymentFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/var/www/site1/" + filesystem.DirMetaFileName: deployment.ActionDirModify,
			},
			rawFileContent: map[str.LocalRepoPath][]byte{
				"host1/var/www/site1/" + filesystem.DirMetaFileName: []byte(`#|^^^|#
{
  "FileOwnerGroup": "root:www-data",
  "FilePermissions": 775,
  "Install": [
    "apt-get install nginx -y"
  ],
  "Preapply": [
    "ss -taplnu | grep 443"
  ],
  "Reload": [
    "systemctl restart php8.3-fpm",
	"systemctl is-active php8.3-fpm"
  ]
}
#|^^^|#
`),
			},
			expectedallFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/var/www/site1/" + filesystem.DirMetaFileName: {
					Hash:             deployment.EmptyFileHash,
					TargetFilePath:   "/var/www/site1",
					RepoFilePath:     "host1/var/www/site1/" + filesystem.DirMetaFileName,
					Action:           deployment.ActionDirModify,
					OwnerGroup:       "root:www-data",
					Permissions:      775,
					InstallOptional:  true,
					Install:          []string{"apt-get install nginx -y"},
					PreapplyRequired: true,
					Preapply:         []string{"ss -taplnu | grep 443"},
					ReloadRequired:   true,
					Reload:           []string{"systemctl restart php8.3-fpm", "systemctl is-active php8.3-fpm"},
				},
			},
			expectedallFileData: map[str.FileID][]byte{"": {}},
			expectedErr:         false,
		},
		{
			name: "Standard delete input",
			allDeploymentFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/exm.conf": deployment.ActionFileDelete,
			},
			rawFileContent: map[str.LocalRepoPath][]byte{
				"host1/etc/exm.conf": {},
			},
			expectedallFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/exm.conf": {
					TargetFilePath: "/etc/exm.conf",
					RepoFilePath:   "host1/etc/exm.conf",
					Action:         deployment.ActionFileDelete,
				},
			},
			expectedallFileData: map[str.FileID][]byte{},
			expectedErr:         false,
		},
		{
			name:                "No input",
			allDeploymentFiles:  map[str.LocalRepoPath]str.DeployAction{},
			rawFileContent:      map[str.LocalRepoPath][]byte{},
			expectedallFileMeta: map[str.LocalRepoPath]deployment.FileInfo{},
			expectedallFileData: map[str.FileID][]byte{},
			expectedErr:         true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			deployFiles, err := ParseFileContent(ctx, test.allDeploymentFiles, test.rawFileContent)

			if err != nil && !test.expectedErr {
				t.Fatalf("Expected no error - but got error '%v'", err)
			}
			if err == nil && test.expectedErr {
				t.Fatalf("Expected err '%v' - but got no error", test.expectedErr)
			}

			for file := range test.allDeploymentFiles {
				gotInfo := deployFiles.GetFileInfo(file)
				gotData := deployFiles.GetFileData(gotInfo.Hash)

				expectedInfo := test.expectedallFileMeta[file]
				expectedData := test.expectedallFileData[expectedInfo.Hash]

				if !reflect.DeepEqual(gotInfo, expectedInfo) {
					t.Errorf("File %s: Metadata mismatch:\nGot:      %#v\nExpected: %#v", file, gotInfo, expectedInfo)
				}
				if !slices.Equal(gotData, expectedData) {
					t.Errorf("File %s: Data mismatch:\nGot:      %s\nExpected: %s", file, string(gotData), string(expectedData))
				}
			}
		})
	}
}
