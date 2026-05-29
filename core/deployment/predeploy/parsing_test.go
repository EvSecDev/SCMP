package predeploy

import (
	"context"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"testing"
)

func TestTranslateLocalPathtoRemotePath(t *testing.T) {
	var cfg config.Config
	cfg.RepositoryPath = "/home/user/repo"
	cfg.UniversalDirectory = "Universal"
	cfg.HostInfo = map[str.RepoRootDir]config.EndpointInfo{
		"868_host_region1": {},
		"host":             {},
		"host1":            {},
		"host2":            {},
		"host3":            {},
		"host4":            {},
		"host9":            {},
		"!@#$%^&*()_+":     {},
	}
	cfg.AllUniversalGroups = map[str.RepoRootDir][]str.RepoRootDir{
		"Universal_VMs": {"host4", "host9"},
	}

	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())

	ctx = context.WithValue(ctx, global.ConfKey, cfg)

	tests := []struct {
		localRepoPath    str.LocalRepoPath
		expectedHostDir  str.RepoRootDir
		expectedFilePath str.RemotePath
	}{
		{"host4/etc/nginx/nginx.conf", "host4", "/etc/nginx/nginx.conf"},
		{"host9/etc/some dir/File Number 1", "host9", "/etc/some dir/File Number 1"},
		{"host/dir/file.txt", "host", "/dir/file.txt"},
		{"host2/dir/subdir/file.txt", "host2", "/dir/subdir/file.txt"},
		{"Universal/etc/resolv.conf", "Universal", "/etc/resolv.conf"},
		{"Universal_VMs/etc/modules.d/01load", "Universal_VMs", "/etc/modules.d/01load"},
		{"../../otherdir/dir/targetfile", "", ""},
		{"file1.txt", "", ""},
		{"dir/", "", ""},
		{"", "", ""},
		{"/", "", ""},
		{"\\", "", ""},
		{"host3/dir/pic.jpeg.remote-artifact", "host3", "/dir/pic.jpeg"},
		{"/home/user/repo/host1/file", "host1", "/file"},
		{"/home/user/repo/host3/etc/service1/target", "host3", "/etc/service1/target"},
		{"!@#$%^&*()_+/etc/file", "!@#$%^&*()_+", "/etc/file"},
	}

	for _, test := range tests {
		t.Run(string(test.localRepoPath), func(t *testing.T) {
			hostDir, targetFilePath := translateLocalPathtoRemotePath(ctx, test.localRepoPath)
			if hostDir != test.expectedHostDir {
				t.Errorf("expected hostDir '%s', got '%s'", test.expectedHostDir, hostDir)
			}
			if targetFilePath != test.expectedFilePath {
				t.Errorf("expected targetFilePath '%s', got '%s'", test.expectedFilePath, targetFilePath)
			}
		})
	}
}
