package repository

import (
	"context"
	"maps"
	"scmp/core/deployment"
	"scmp/core/filesystem"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/filemode"
)

func TestParseChangedFiles(t *testing.T) {
	// Mock Globals
	var cfg config.Config
	cfg.HostInfo = map[str.RepoRootDir]config.EndpointInfo{
		"host1": {},
		"host2": {},
		"host3": {},
		"host4": {},
	}

	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())
	ctx = context.WithValue(ctx, global.ConfKey, cfg)

	type TestCase struct {
		name                string
		changedFiles        []GitChangedFileMetadata
		allowDeletions      bool
		fileOverride        string
		expectedCommitFiles map[str.LocalRepoPath]str.DeployAction
	}
	testCases := []TestCase{
		{
			name: "Single - New File",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "",
					fromMode:    filemode.FileMode(0),
					toNotOnFS:   false,
					toPath:      "host1/etc/network/interfaces",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/network/interfaces": deployment.ActionFileCreate,
			},
		},
		{
			name: "Single - New Dir Meta",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "",
					fromMode:    filemode.FileMode(0),
					toNotOnFS:   false,
					toPath:      str.LocalRepoPath("host1/var/www/site/" + filesystem.DirMetaFileName),
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/var/www/site/" + filesystem.DirMetaFileName: deployment.ActionDirCreate,
			},
		},
		{
			name: "Single - Modified Dir Meta",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    str.LocalRepoPath("host2/opt/prog/" + filesystem.DirMetaFileName),
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      str.LocalRepoPath("host2/opt/prog/" + filesystem.DirMetaFileName),
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host2/opt/prog/" + filesystem.DirMetaFileName: deployment.ActionDirModify,
			},
		},
		{
			name: "Single - Moved to another host with deletions",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "host1/etc/network/interfaces",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host2/etc/network/interfaces",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			allowDeletions: true,
			fileOverride:   "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/network/interfaces": deployment.ActionFileDelete,
				"host2/etc/network/interfaces": deployment.ActionFileCreate,
			},
		},
		{
			name: "Single - Moved to another host",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "host1/etc/network/interfaces",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host2/etc/network/interfaces",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host2/etc/network/interfaces": deployment.ActionFileCreate,
			},
		},
		{
			name: "Multiple - User override",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host2/etc/hostname",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   true,
					toPath:      "host2/etc/hostname",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
				{
					fromNotOnFS: false,
					fromPath:    "host3/etc/resolv.conf",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host3/etc/resolv.conf",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
				{
					fromNotOnFS: false,
					fromPath:    "host4/etc/rsyslog.conf",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host4/etc/rsyslog.conf",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			fileOverride: "host3/etc/resolv.conf",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host3/etc/resolv.conf": deployment.ActionFileModify,
			},
		},
		{
			name: "Single - Same Name",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host1/etc/hosts",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host1/etc/hosts",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/hosts": deployment.ActionFileModify,
			},
		},
		{
			name: "Single - Copied to Other Host",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host1/etc/default/grub",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host3/etc/default/grub",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host3/etc/default/grub": deployment.ActionFileCreate,
			},
		},
		{
			name: "Dual - Rename and In-Place",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "host1/etc/hosts",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host1/etc/backup.hosts",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
				{
					fromNotOnFS: false,
					fromPath:    "host2/etc/conf1",
					fromMode:    filemode.FileMode(uint32(0100644)),
					toNotOnFS:   false,
					toPath:      "host2/etc/conf1",
					toMode:      filemode.FileMode(uint32(0100644)),
				},
			},
			allowDeletions: true,
			fileOverride:   "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/hosts":        deployment.ActionFileDelete,
				"host1/etc/backup.hosts": deployment.ActionFileCreate,
				"host2/etc/conf1":        deployment.ActionFileModify,
			},
		},
		{
			name: "Modified Unsupported File Type",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host4/dev/sda",
					fromMode:    filemode.FileMode(uint32(0100664)),
					toNotOnFS:   false,
					toPath:      "host4/dev/sda",
					toMode:      filemode.FileMode(uint32(0100664)),
				},
			},
			fileOverride:        "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{},
		},
		{
			name:                "No input",
			changedFiles:        []GitChangedFileMetadata{},
			fileOverride:        "",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{},
		},
		{
			name:                "Only override input",
			changedFiles:        []GitChangedFileMetadata{},
			fileOverride:        "host1/etc/file.conf,host2/etc/conf.file",
			expectedCommitFiles: map[str.LocalRepoPath]str.DeployAction{},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx = context.WithValue(ctx, global.OpsKey, config.Opts{AllowDeletions: test.allowDeletions})

			commitFiles := ParseChangedFiles(ctx, test.changedFiles, test.fileOverride)

			if !maps.Equal(test.expectedCommitFiles, commitFiles) {
				t.Errorf("Expected metadata does not match output metadata:\nOutput:\n%#v\n\nExpected Output:\n%#v\n", commitFiles, test.expectedCommitFiles)
			}
		})
	}
}

func TestMapFilesByHostOrUniversal(t *testing.T) {
	// Initialize global config
	config := config.Config{
		UniversalDirectory: "universal",
		AllUniversalGroups: map[str.RepoRootDir][]str.RepoRootDir{
			"universalGroup1": {"host9"},
			"universalGroup2": {"host11"},
		},
	}

	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())
	ctx = context.WithValue(ctx, global.ConfKey, config)

	// Test cases
	tests := []struct {
		name                   string
		allRepoFiles           []string
		expectedHostFiles      map[str.RepoRootDir]map[str.RemotePath]struct{}
		expectedUniversalFiles map[str.RepoRootDir]map[str.RemotePath]struct{}
	}{
		{
			name:         "Check for map clobbering",
			allRepoFiles: []string{"universal/some/other/file.txt", "universal/some/file2.txt", "hostDir/some/host/file.txt", "hostDir/some/file2.txt"},
			expectedHostFiles: map[str.RepoRootDir]map[str.RemotePath]struct{}{
				"hostDir": {
					"some/host/file.txt": {},
					"some/file2.txt":     {},
				},
			},
			expectedUniversalFiles: map[str.RepoRootDir]map[str.RemotePath]struct{}{
				"universal": {
					"some/other/file.txt": {},
					"some/file2.txt":      {},
				},
			},
		},
		{
			name:              "File in universal directory",
			allRepoFiles:      []string{"universal/some/other/file.txt"},
			expectedHostFiles: map[str.RepoRootDir]map[str.RemotePath]struct{}{},
			expectedUniversalFiles: map[str.RepoRootDir]map[str.RemotePath]struct{}{
				"universal": {
					"some/other/file.txt": {},
				},
			},
		},
		{
			name:         "File in host directory",
			allRepoFiles: []string{"hostDir/some/host/file.txt"},
			expectedHostFiles: map[str.RepoRootDir]map[str.RemotePath]struct{}{
				"hostDir": {
					"some/host/file.txt": {},
				},
			},
			expectedUniversalFiles: map[str.RepoRootDir]map[str.RemotePath]struct{}{},
		},
		{
			name:                   "File at root (ignored)",
			allRepoFiles:           []string{"file_at_root.txt"},
			expectedHostFiles:      map[str.RepoRootDir]map[str.RemotePath]struct{}{},
			expectedUniversalFiles: map[str.RepoRootDir]map[str.RemotePath]struct{}{},
		},
	}

	// Run tests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Setup initial maps
			allHostsFiles := make(map[str.RepoRootDir]map[str.RemotePath]struct{})
			allUniversalFiles := make(map[str.RepoRootDir]map[str.RemotePath]struct{})

			for _, testRepoFile := range test.allRepoFiles {
				// Call the function under test
				mapFilesByHostOrUniversal(ctx, testRepoFile, allHostsFiles, allUniversalFiles)
			}

			// Validate results
			if !str.EqualMaps(allHostsFiles, test.expectedHostFiles) {
				t.Errorf("Expected host files %v, but got %v", test.expectedHostFiles, allHostsFiles)
			}

			if !str.EqualMaps(allUniversalFiles, test.expectedUniversalFiles) {
				t.Errorf("Expected universal files %v, but got %v", test.expectedUniversalFiles, allUniversalFiles)
			}
		})
	}
}
