package repository

import (
	"context"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"testing"
)

func TestValidateRepoFile(t *testing.T) {
	// Mock global.for the tests
	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())
	var cfg config.Config
	cfg.HostInfo = make(map[str.RepoRootDir]config.EndpointInfo)
	cfg.HostInfo["validHost"] = config.EndpointInfo{EndpointName: "validHost"}
	cfg.HostInfo["validHost2"] = config.EndpointInfo{EndpointName: "validHost2"}
	cfg.UniversalDirectory = "UniversalConfs"
	cfg.AllUniversalGroups = map[str.RepoRootDir][]str.RepoRootDir{
		"UniversalConfs_Group1": {"host14"},
	}
	ctx = context.WithValue(ctx, global.ConfKey, cfg)

	tests := []struct {
		path     str.LocalRepoPath
		expected struct {
			skipFile bool
		}
	}{
		{"file.txt", struct {
			skipFile bool
		}{true}},
		{deployment.IgnoreDirectoryPrefix + "ignoreDir/file.txt", struct {
			skipFile bool
		}{true}},
		{"validHost/etc/file.txt", struct {
			skipFile bool
		}{false}},
		{"UniversalConfs/file.txt", struct {
			skipFile bool
		}{false}},
		{"UniversalConfs_Group1/file.txt", struct {
			skipFile bool
		}{false}},
		{"invalidDir/file.txt", struct {
			skipFile bool
		}{true}},
		{"/etc/file.txt", struct {
			skipFile bool
		}{true}},
		{"", struct {
			skipFile bool
		}{true}},
	}

	for _, test := range tests {
		t.Run(string(test.path), func(t *testing.T) {
			skipFile := repoFileIsNotValid(ctx, test.path)
			if skipFile != test.expected.skipFile {
				t.Errorf("expected skipFile to be %t, got %t", test.expected.skipFile, skipFile)
			}
		})
	}
}
