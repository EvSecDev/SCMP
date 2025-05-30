// controller
package main

import (
	"fmt"
	"testing"
)

func TestExtractMetadata(t *testing.T) {
	tests := []struct {
		name                     string
		fileContents             string
		expectedMetadata         MetaHeader
		expectedRemainingContent string
		expectedError            error
	}{
		{
			name: "Valid Metadata",
			fileContents: `#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755,
  "Dependencies": [
    "Host1/etc/network/interfaces",
	"Host1/etc/hosts"
  ],
  "Install": [
    "command0",
	"command3"
  ],
  "Checks": [
    "check1",
	"check2"
  ],
  "Reload": [
    "command1",
    "command2"
  ]
}
#|^^^|#
file content
file content
file content
`,
			expectedMetadata: MetaHeader{
				TargetFileOwnerGroup:  "root:root",
				TargetFilePermissions: 755,
				Dependencies:          []string{"Host1/etc/network/interfaces", "Host1/etc/hosts"},
				InstallCommands:       []string{"command0", "command3"},
				CheckCommands:         []string{"check1", "check2"},
				ReloadCommands:        []string{"command1", "command2"},
			},
			expectedRemainingContent: `file content
file content
file content
`,
			expectedError: nil,
		},
		{
			name: "Valid Metadata 2",
			fileContents: `#|^^^|#
#{
#  "FileOwnerGroup": "root:root",
#  "FilePermissions": 755,
#  "Reload": [
#    "command1",
#    "command2"
#  ]
#}
#|^^^|#
file content file content file content
`,
			expectedMetadata: MetaHeader{
				TargetFileOwnerGroup:  "root:root",
				TargetFilePermissions: 755,
				ReloadCommands:        []string{"command1", "command2"},
			},
			expectedRemainingContent: `file content file content file content
`,
			expectedError: nil,
		},
		{
			name: "Valid Metadata 3",
			fileContents: `;#|^^^|#
;{
;  "FileOwnerGroup": "root:root",
;  "FilePermissions": 755,
;  "Reload": [
;    "command1",
;    "command2"
;  ]
;}
;#|^^^|#
file content
file content file content
`,
			expectedMetadata: MetaHeader{
				TargetFileOwnerGroup:  "root:root",
				TargetFilePermissions: 755,
				ReloadCommands:        []string{"command1", "command2"},
			},
			expectedRemainingContent: `file content
file content file content
`,
			expectedError: nil,
		},
		{
			name: "Multiline Commented Header",
			fileContents: `/*#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755
}#|^^^|#*/
file content
file content
file content
`,
			expectedMetadata: MetaHeader{
				TargetFileOwnerGroup:  "root:root",
				TargetFilePermissions: 755,
			},
			expectedRemainingContent: `file content
file content
file content
`,
			expectedError: nil,
		},
		{
			name: "Multiline Commented Header 2",
			fileContents: `<!--#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755
}
#|^^^|#-->
file content
file content
file content
`,
			expectedMetadata: MetaHeader{
				TargetFileOwnerGroup:  "root:root",
				TargetFilePermissions: 755,
			},
			expectedRemainingContent: `file content
file content
file content
`,
			expectedError: nil,
		},
		{
			name: "No Newline After End Delimiter",
			fileContents: `#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755
}
#|^^^|#thisissomefilecontent`,
			expectedMetadata: MetaHeader{
				TargetFileOwnerGroup:  "root:root",
				TargetFilePermissions: 755,
			},
			expectedRemainingContent: "thisissomefilecontent",
			expectedError:            nil,
		},
		{
			name: "No End Delimiter",
			fileContents: `#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755
}
file content file content file content`,
			expectedMetadata:         MetaHeader{},
			expectedRemainingContent: "",
			expectedError:            fmt.Errorf("json end delimiter missing"),
		},
		{
			name:                     "Missing Start Delimiter",
			fileContents:             `file content file content file content`,
			expectedMetadata:         MetaHeader{},
			expectedRemainingContent: "",
			expectedError:            fmt.Errorf("json start delimiter missing"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, remaining, err := extractMetadata(test.fileContents)

			if test.expectedError != nil {
				if err == nil {
					t.Errorf("expected error '%v', but got nil", test.expectedError)
				} else if err.Error() != test.expectedError.Error() {
					t.Errorf("expected error '%v', got '%v'", test.expectedError, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, but got '%v'", err)
			}

			if err == nil {
				if fmt.Sprintf("%v", metadata) != fmt.Sprintf("%v", test.expectedMetadata) {
					t.Errorf("expected metadata '%v', got '%v'", test.expectedMetadata, metadata)
				}
				if string(remaining) != test.expectedRemainingContent {
					t.Errorf("expected remaining content '%v', got '%v'", test.expectedRemainingContent, string(remaining))
				}
			}
		})
	}
}
