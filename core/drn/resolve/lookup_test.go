package resolve

import (
	"context"
	"os"
	"path/filepath"
	"scmp/core/drn"
	"scmp/core/drn/drnconfig"
	glbConfig "scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"testing"
)

func TestLookupValue(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	cfg := glbConfig.Config{
		RepositoryPath: tmpDir,
		HostInfo: map[str.RepoRootDir]glbConfig.EndpointInfo{
			"host1": {
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
		},
	}
	ctx = context.WithValue(ctx, global.ConfKey, cfg)

	// Create an external config
	ncDir := filepath.Join(tmpDir, drn.ExternalVariableDirectory)
	err := os.MkdirAll(ncDir, 0700)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	node := drnconfig.MapToCfgNode(map[string]string{"host" + drn.FieldSeparator + "type": "production"})
	err = node.WriteConfig(filepath.Join(ncDir, "myconfig"))
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	tests := []struct {
		name        string
		drn         string
		ctxPath     str.LocalRepoPath
		ctxHost     str.RepoRootDir
		expect      str.DRNVal
		expectError string
	}{
		{
			name:        "Internal DRN without context fails",
			drn:         drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "host", "alias"),
			ctxPath:     "",
			ctxHost:     "",
			expectError: "provided internal DRN requires context host alias",
		},
		{
			name:    "Internal DRN with context succeeds (file name macro)",
			drn:     drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "repo", "file", "name"),
			ctxPath: "host1/etc/nginx.conf",
			ctxHost: "host1",
			expect:  "nginx.conf",
		},
		{
			name:    "Internal DRN with context succeeds (host alias)",
			drn:     drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "host", "alias"),
			ctxPath: "host1/etc/nginx.conf",
			ctxHost: "host1",
			expect:  "host1",
		},
		{
			name:    "Internal DRN with context succeeds (host address)",
			drn:     drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "host", "net", "address"),
			ctxPath: "host1/etc/nginx.conf",
			ctxHost: "host1",
			expect:  "127.0.0.1",
		},
		{
			name:    "Internal DRN with context succeeds (repo file path)",
			drn:     drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "repo", "file", "path"),
			ctxPath: "host1/etc/nginx.conf",
			ctxHost: "host1",
			expect:  "/etc/nginx.conf",
		},
		{
			name:    "External DRN direct lookup (no context)",
			drn:     drn.QuickFormat([]string{"myconfig"}, "host", "type"),
			ctxPath: "",
			ctxHost: "",
			expect:  "production",
		},
		{
			name:    "External DRN with context",
			drn:     drn.QuickFormat([]string{"myconfig"}, "host", "type"),
			ctxPath: "host1/etc/nginx.conf",
			ctxHost: "host1",
			expect:  "production",
		},
		{
			name:        "Invalid DRN",
			drn:         "not-a-drn",
			ctxPath:     "",
			ctxHost:     "",
			expectError: drn.ErrNotDRN.Error(),
		},
		{
			name:        "Unknown host alias",
			drn:         drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "host", "alias"),
			ctxPath:     "host1/etc/nginx.conf",
			ctxHost:     "unknown",
			expectError: "unknown host alias",
		},
		{
			name:        "External config namespace not found",
			drn:         drn.QuickFormat([]string{"nonexist"}, "host", "type"),
			ctxPath:     "",
			ctxHost:     "",
			expectError: "config load",
		},
		{
			name:        "External field not found",
			drn:         drn.QuickFormat([]string{"myconfig"}, "host", "missing"),
			ctxPath:     "",
			ctxHost:     "",
			expectError: "field 'missing' not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			val, err := LookupValue(ctx, test.drn, test.ctxPath, test.ctxHost)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if val != test.expect {
				t.Errorf("got %q, expected %q", val, test.expect)
			}
		})
	}
}
