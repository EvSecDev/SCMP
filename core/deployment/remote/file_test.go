package remote

import (
	"context"
	"os"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"testing"
)

func TestCheckForDiff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done()) // New logger tied to global
	logger := logctx.GetLogger(ctx)                                        // Add logger to global ctx
	logger.SetFormattedOutput(os.Stdout)                                   // Send received output to stdout
	logctx.StartOutput(ctx)
	ctx = context.WithValue(ctx, global.OpsKey, config.Opts{ForceEnabled: false})

	tests := []struct {
		name                    string
		remoteMetadata          sshinternal.RemoteFileInfo
		localMetadata           deployment.FileInfo
		expectedContentDiffers  bool
		expectedMetadataDiffers bool
	}{
		{
			name: "Everything differs",
			remoteMetadata: sshinternal.RemoteFileInfo{
				Hash:        "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
				Permissions: 757,
				Owner:       "user1",
				Group:       "group1",
			},
			localMetadata: deployment.FileInfo{
				Hash:        "590c9f8430c7435807df8ba9a476e3f1295d46ef210f6efae2043a4c085a569e",
				Permissions: 640,
				OwnerGroup:  "user2:group1",
			},
			expectedContentDiffers:  true,
			expectedMetadataDiffers: true,
		},
		{
			name: "Hashes differ",
			remoteMetadata: sshinternal.RemoteFileInfo{
				Hash: "1b4f0e9851971998e732078544c96b36c3d01cedf7caa332359d6f1d83567014",
			},
			localMetadata: deployment.FileInfo{
				Hash: "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
			},
			expectedContentDiffers:  true,
			expectedMetadataDiffers: false,
		},
		{
			name: "Permissions differ",
			remoteMetadata: sshinternal.RemoteFileInfo{
				Permissions: 757,
			},
			localMetadata: deployment.FileInfo{
				Permissions: 640,
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: true,
		},
		{
			name: "Owner and group differ",
			remoteMetadata: sshinternal.RemoteFileInfo{
				Owner: "user1",
				Group: "group1",
			},
			localMetadata: deployment.FileInfo{
				OwnerGroup: "user2:group2",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: true,
		},
		{
			name: "No differences",
			remoteMetadata: sshinternal.RemoteFileInfo{
				Hash:        "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
				Permissions: 0755,
				Owner:       "user1",
				Group:       "group1",
			},
			localMetadata: deployment.FileInfo{
				Hash:        "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
				Permissions: 0755,
				OwnerGroup:  "user1:group1",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: false,
		},
		{
			name: "No data",
			remoteMetadata: sshinternal.RemoteFileInfo{
				Hash:        "",
				Permissions: 0,
				Owner:       "",
				Group:       "",
			},
			localMetadata: deployment.FileInfo{
				Hash:        "",
				Permissions: 0,
				OwnerGroup:  "",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contentDiffers, metadataDiffers := CheckForDiff(ctx, test.remoteMetadata, test.localMetadata)

			if contentDiffers != test.expectedContentDiffers {
				t.Errorf("expected contentDiffers %v, got %v", test.expectedContentDiffers, contentDiffers)
			}
			if metadataDiffers != test.expectedMetadataDiffers {
				t.Errorf("expected metadataDiffers %v, got %v", test.expectedMetadataDiffers, metadataDiffers)
			}
		})
	}
}
