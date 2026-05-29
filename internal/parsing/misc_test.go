package parsing

import (
	"context"
	"fmt"
	"os"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"testing"
)

func TestCheckForOverride(t *testing.T) {
	// Mock globals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done()) // New logger tied to global
	logger := logctx.GetLogger(ctx)                                        // Add logger to global ctx
	logger.SetFormattedOutput(os.Stdout)                                   // Send received output to stdout
	logctx.StartOutput(ctx)

	cfg := config.Config{
		AllUniversalGroups: map[str.RepoRootDir][]str.RepoRootDir{
			"universalGroup1": {"host9"},
			"universalGroup2": {"host11"},
		},
		HostInfo: map[str.RepoRootDir]config.EndpointInfo{
			"host1": {
				UniversalGroups: map[str.RepoRootDir]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host2": {
				UniversalGroups: map[str.RepoRootDir]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host3": {
				UniversalGroups: map[str.RepoRootDir]struct{}{
					"": {},
				},
			},
		},
		UniversalDirectory: "universalconfs",
	}

	// Test cases
	tests := []struct {
		override     string
		current      string
		expectedSkip bool
		useRegex     bool
	}{
		{"", "host1", false, false},
		{"host1", "host1", false, false},
		{"host1,host2", "host1", false, false},
		{"host1,host2", "host3", true, false},
		{"host1, host2", "host3", true, false},
		{"host1, host2, host3, host4, host5, host6", "host3", true, false},
		{"file1.txt,file2.txt", "file1.txt", false, false},
		{"file1.txt,file2.txt", "file3.txt", true, false},
		{"file!@%$^&*(4.txt,file6.txt", "file6.txt", false, false},
		{"file!@%$^&*(4.txt,file6.txt", "file!@%$^&*(4.txt", false, false},
		{"universalconfs/.*", "universalconfs/etc/hosts", false, true},
		{"universalconfs/etc/", "universalconfs/var/log/file.txt", true, true},
		{"universalconfs/.*", "universalconfs_ssh/etc/ssh/sshd_config", true, true},
		{"dc0[0-9].*etc/network/interfaces", "region1_dc02_host321/etc/network/interfaces", false, true},
		{"(?=\\d{3}-\\d{2}-\\d{4})\\d{3}-\\d{2}-\\d{4}", "123-45-6789", false, true},
		{"(\\d+)\\s+", "1234abc", true, true},
		{"host0*", "host0436", false, true},
		{"UniversalConfs_Service1", "host2", false, false},
		{"UniversalConfs_Service1", "host3", true, false},
	}

	for _, test := range tests {
		// Mock global for this test
		var opts config.Opts
		opts.RegexEnabled = test.useRegex
		ctx = context.WithValue(ctx, global.OpsKey, opts)

		testTitle := fmt.Sprintf("Available Items:'%s'-Current Item:'%s'", test.override, test.current)
		t.Run(testTitle, func(t *testing.T) {
			skip := CheckForOverride(ctx, test.override, test.current, cfg.HostInfo)
			if skip != test.expectedSkip {
				t.Errorf("Skip current item? %t; Should skip current item? %t", skip, test.expectedSkip)
			}
		})
	}
}
