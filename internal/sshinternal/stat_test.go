package sshinternal

import (
	"testing"
)

func TestExtractMetadataFromStat(t *testing.T) {
	tests := []struct {
		name        string
		statOutput  string
		expected    RemoteFileInfo
		expectError bool
	}{
		{
			name:       "normal file",
			statOutput: "[/etc/rmt],[regular file],[root],[root],[640],[53],['/etc/rmt']",
			expected: RemoteFileInfo{
				Name:        "/etc/rmt",
				FsType:      "regular file",
				Owner:       "root",
				Group:       "root",
				Permissions: 640,
				Size:        53,
				LinkTarget:  "",
				Exists:      true,
			},
			expectError: false,
		},
		{
			name:       "normal file second format",
			statOutput: "[/etc/RMT],[Regular file],[root],[root],[0640],[4853],['/etc/rmt']",
			expected: RemoteFileInfo{
				Name:        "/etc/RMT",
				FsType:      "regular file",
				Owner:       "root",
				Group:       "root",
				Permissions: 640,
				Size:        4853,
				LinkTarget:  "",
				Exists:      true,
			},
			expectError: false,
		},
		{
			name:       "normal file bsd",
			statOutput: "[/usr/local/etc/FILE1],[Regular file],[root],[wheel],[644],[254],[target=]",
			expected: RemoteFileInfo{
				Name:        "/usr/local/etc/FILE1",
				FsType:      "regular file",
				Owner:       "root",
				Group:       "wheel",
				Permissions: 644,
				Size:        254,
				LinkTarget:  "",
				Exists:      true,
			},
			expectError: false,
		},
		{
			name:       "symbolic link bsd",
			statOutput: "[/etc/File1],[Symbolic Link],[root],[root],[0755],[11],[target=/etc/conf/FILE2]",
			expected: RemoteFileInfo{
				Name:        "/etc/File1",
				FsType:      "symbolic link",
				Owner:       "root",
				Group:       "root",
				Permissions: 755,
				Size:        11,
				LinkTarget:  "/etc/conf/FILE2",
				Exists:      true,
			},
			expectError: false,
		},
		{
			name:        "invalid field count",
			statOutput:  "[/etc/rmt],[symbolic link],[root],[root],[777],[13]",
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name:        "incorrect field prefix",
			statOutput:  "etc/rmt],[symbolic link],[root],[root],[777],[13],['/etc/rmt' -> '/usr/sbin/rmt']",
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name:        "incorrect field suffix",
			statOutput:  "[/etc/rmt],[symbolic link],[root],[root],[777],[13],['/etc/rmt' -> '/usr/sbin/rmt'",
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name:        "invalid permission bits",
			statOutput:  "[/etc/rmt],[symbolic link],[root],[root],[abc],[13],['/etc/rmt' -> '/usr/sbin/rmt']",
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name:        "invalid file size",
			statOutput:  "[/etc/rmt],[symbolic link],[root],[root],[777],[xyz],['/etc/rmt' -> '/usr/sbin/rmt']",
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name:        "nothing",
			statOutput:  "",
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name: "newline nothing",
			statOutput: `
			`,
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name: "newline filename",
			statOutput: `[/etc/rmt
file],[regular file],[root],[root],[777],[584938593485983],[]`,
			expected:    RemoteFileInfo{},
			expectError: true,
		},
		{
			name: "no symlink with newline after command",
			statOutput: `[/etc/rmt],[regular file],[root],[root],[777],[1024],[]
`,
			expected: RemoteFileInfo{
				Name:        "/etc/rmt",
				FsType:      "regular file",
				Owner:       "root",
				Group:       "root",
				Permissions: 777,
				Size:        1024,
				LinkTarget:  "",
				Exists:      true,
			},
			expectError: false,
		},
		{
			name:       "symlink",
			statOutput: "[/etc/r mt],[regular file],[root],[root],[777],[1024],['/etc/r mt' -> '/usr/sbin/rm t']",
			expected: RemoteFileInfo{
				Name:        "/etc/r mt",
				FsType:      "regular file",
				Owner:       "root",
				Group:       "root",
				Permissions: 777,
				Size:        1024,
				LinkTarget:  "/usr/sbin/rm t",
				Exists:      true,
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fileInfo, err := ExtractMetadataFromStat(test.statOutput)

			if test.expectError {
				if err == nil {
					t.Fatalf("expected error, but got none")
				}
				return // if we expect an error, no further checks are needed
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Manually check for equality
			if fileInfo.Name != test.expected.Name {
				t.Errorf("expected name: '%s', got: '%s'", test.expected.Name, fileInfo.Name)
			}
			if fileInfo.FsType != test.expected.FsType {
				t.Errorf("expected FsType: '%s', got: '%s'", test.expected.FsType, fileInfo.FsType)
			}
			if fileInfo.Owner != test.expected.Owner {
				t.Errorf("expected Owner: '%s', got: '%s'", test.expected.Owner, fileInfo.Owner)
			}
			if fileInfo.Group != test.expected.Group {
				t.Errorf("expected Group: '%s', got: '%s'", test.expected.Group, fileInfo.Group)
			}
			if fileInfo.Permissions != test.expected.Permissions {
				t.Errorf("expected Permissions: '%d', got: '%d'", test.expected.Permissions, fileInfo.Permissions)
			}
			if fileInfo.Size != test.expected.Size {
				t.Errorf("expected Size: '%d', got: '%d'", test.expected.Size, fileInfo.Size)
			}
			if fileInfo.LinkTarget != test.expected.LinkTarget {
				t.Errorf("expected LinkTarget: '%s', got: '%s'", test.expected.LinkTarget, fileInfo.LinkTarget)
			}
			if fileInfo.Exists != test.expected.Exists {
				t.Errorf("expected Exists: '%t', got: '%t'", test.expected.Exists, fileInfo.Exists)
			}
		})
	}
}
