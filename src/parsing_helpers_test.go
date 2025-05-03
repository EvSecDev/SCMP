// controller
package main

import (
	"fmt"
	"testing"
)

// Unit test for checkForOverride
func TestCheckForOverride(t *testing.T) {
	// Mock globals
	config = Config{
		allUniversalGroups: map[string][]string{
			"universalGroup1": {"host9"},
			"universalGroup2": {"host11"},
		},
		hostInfo: map[string]EndpointInfo{
			"host1": {
				universalGroups: map[string]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host2": {
				universalGroups: map[string]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host3": {
				universalGroups: map[string]struct{}{
					"": {},
				},
			},
		},
		universalDirectory: "universalconfs",
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
		config.options.regexEnabled = test.useRegex

		testTitle := fmt.Sprintf("Available Items:'%s'-Current Item:'%s'", test.override, test.current)
		t.Run(testTitle, func(t *testing.T) {
			skip := checkForOverride(test.override, test.current)
			if skip != test.expectedSkip {
				t.Errorf("Skip current item? %t; Should skip current item? %t", skip, test.expectedSkip)
			}
		})
	}
}

func TestMapFilesByHostOrUniversal(t *testing.T) {
	// Initialize global config
	config = Config{
		osPathSeparator:    "/",
		universalDirectory: "universal",
		allUniversalGroups: map[string][]string{
			"universalGroup1": {"host9"},
			"universalGroup2": {"host11"},
		},
	}

	// Test cases
	tests := []struct {
		name                   string
		allRepoFiles           []string
		expectedHostFiles      map[string]map[string]struct{}
		expectedUniversalFiles map[string]map[string]struct{}
	}{
		{
			name:         "Check for map clobbering",
			allRepoFiles: []string{"universal/some/other/file.txt", "universal/some/file2.txt", "hostDir/some/host/file.txt", "hostDir/some/file2.txt"},
			expectedHostFiles: map[string]map[string]struct{}{
				"hostDir": {
					"some/host/file.txt": {},
					"some/file2.txt":     {},
				},
			},
			expectedUniversalFiles: map[string]map[string]struct{}{
				"universal": {
					"some/other/file.txt": {},
					"some/file2.txt":      {},
				},
			},
		},
		{
			name:              "File in universal directory",
			allRepoFiles:      []string{"universal/some/other/file.txt"},
			expectedHostFiles: map[string]map[string]struct{}{},
			expectedUniversalFiles: map[string]map[string]struct{}{
				"universal": {
					"some/other/file.txt": {},
				},
			},
		},
		{
			name:         "File in host directory",
			allRepoFiles: []string{"hostDir/some/host/file.txt"},
			expectedHostFiles: map[string]map[string]struct{}{
				"hostDir": {
					"some/host/file.txt": {},
				},
			},
			expectedUniversalFiles: map[string]map[string]struct{}{},
		},
		{
			name:                   "File at root (ignored)",
			allRepoFiles:           []string{"file_at_root.txt"},
			expectedHostFiles:      map[string]map[string]struct{}{},
			expectedUniversalFiles: map[string]map[string]struct{}{},
		},
	}

	// Run tests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Setup initial maps
			allHostsFiles := make(map[string]map[string]struct{})
			allUniversalFiles := make(map[string]map[string]struct{})

			for _, testRepoFile := range test.allRepoFiles {
				// Call the function under test
				mapFilesByHostOrUniversal(testRepoFile, allHostsFiles, allUniversalFiles)
			}

			// Validate results
			if !equalMaps(allHostsFiles, test.expectedHostFiles) {
				t.Errorf("Expected host files %v, but got %v", test.expectedHostFiles, allHostsFiles)
			}

			if !equalMaps(allUniversalFiles, test.expectedUniversalFiles) {
				t.Errorf("Expected universal files %v, but got %v", test.expectedUniversalFiles, allUniversalFiles)
			}
		})
	}
}

// Helper function to compare two maps for equality
func equalMaps(a, b map[string]map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}

	for key, aVal := range a {
		bVal, ok := b[key]
		if !ok {
			return false
		}
		if len(aVal) != len(bVal) {
			return false
		}
		for file := range aVal {
			if _, ok := bVal[file]; !ok {
				return false
			}
		}
	}

	return true
}

func TestMapDeniedUniversalFiles(t *testing.T) {
	// Mock Global
	config = Config{
		hostInfo: map[string]EndpointInfo{
			"host1": {
				universalGroups: map[string]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host2": {
				universalGroups: map[string]struct{}{
					"UniversalConfs_OtherServers": {},
				},
			},
			"host3": {
				universalGroups: map[string]struct{}{
					"": {},
				},
			},
		},
		universalDirectory: "UniversalConfs",
	}

	// Test Data
	allHostsFiles := map[string]map[string]struct{}{
		"host1": {
			"etc/file1.txt": {},
			"etc/file2.txt": {},
			"etc/file3.txt": {},
		},
		"host2": {
			"etc/file4.txt": {},
			"etc/file5.txt": {},
			"etc/file6.txt": {},
		},
		"host3": {
			"etc/file7.txt": {},
			"etc/file8.txt": {},
			"etc/file9.txt": {},
		},
	}
	universalFiles := map[string]map[string]struct{}{
		"UniversalConfs_Service1": {
			"etc/file1.txt": {},
			"etc/file3.txt": {},
		},
		"UniversalConfs_OtherServers": {
			"etc/file2.txt": {},
			"etc/file4.txt": {},
		},
		"UniversalConfs": {
			"etc/file5.txt": {},
		},
	}

	// Call the function under test
	deniedUniversalFiles := mapDeniedUniversalFiles(allHostsFiles, universalFiles)

	// Expected result
	expectedDeniedFiles := map[string]map[string]struct{}{
		"host1": {
			"UniversalConfs_Service1/etc/file1.txt": {},
			"UniversalConfs_Service1/etc/file3.txt": {},
		},
		"host2": {
			"UniversalConfs/etc/file5.txt":              {},
			"UniversalConfs_OtherServers/etc/file4.txt": {},
		},
	}

	// Check if the result matches the expected output
	for host, deniedFiles := range expectedDeniedFiles {
		for filePath := range deniedFiles {
			_, fileIsInDenied := deniedUniversalFiles[host][filePath]
			if !fileIsInDenied {
				t.Errorf("For test %s, expected denied file %s, but it was not found", host, filePath)
			}
		}

		// Check for extra denied files in the actual result
		for filePath := range deniedUniversalFiles[host] {
			_, fileIsExpectedDenied := expectedDeniedFiles[host][filePath]
			if !fileIsExpectedDenied {
				t.Errorf("For test %s, found extra denied file %s, which was not expected", host, filePath)
			}
		}
	}
}

func TestExtractMetadata(t *testing.T) {
	tests := []struct {
		name                     string
		fileContents             string
		expectedMetadata         string
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
file content file content file content`,
			expectedMetadata: `
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
`,
			expectedRemainingContent: "file content file content file content",
			expectedError:            nil,
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
file content file content file content`,
			expectedMetadata: `
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755,
  "Reload": [
    "command1",
    "command2"
  ]
}
`,
			expectedRemainingContent: "file content file content file content",
			expectedError:            nil,
		},
		{
			name:                     "Missing Start Delimiter",
			fileContents:             `file content file content file content`,
			expectedMetadata:         "",
			expectedRemainingContent: "",
			expectedError:            fmt.Errorf("json start delimiter missing"),
		},
		{
			name: "No End Delimiter",
			fileContents: `#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755
}
file content file content file content`,
			expectedMetadata:         "",
			expectedRemainingContent: "",
			expectedError:            fmt.Errorf("json end delimiter missing"),
		},
		{
			name: "Missing Newline After End Delimiter",
			fileContents: `#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 755
}
#|^^^|#`,
			expectedMetadata:         "",
			expectedRemainingContent: "",
			expectedError:            fmt.Errorf("json end delimiter missing"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, remaining, err := extractMetadata(test.fileContents)

			// If we expect an error, check that it's not nil and matches the expected error
			if test.expectedError != nil {
				if err == nil {
					t.Errorf("expected error '%v', but got nil", test.expectedError)
				} else if err.Error() != test.expectedError.Error() {
					t.Errorf("expected error '%v', got '%v'", test.expectedError, err)
				}
			} else if err != nil {
				// If no error is expected but we got one, this is a failure
				t.Errorf("expected no error, but got '%v'", err)
			}

			// If no error, check that the metadata and remaining content are correct
			if err == nil {
				if metadata != test.expectedMetadata {
					t.Errorf("expected metadata '%v', got '%v'", test.expectedMetadata, metadata)
				}
				if string(remaining) != test.expectedRemainingContent {
					t.Errorf("expected remaining content '%v', got '%v'", test.expectedRemainingContent, remaining)
				}
			}
		})
	}
}

func TestValidateRepoFile(t *testing.T) {
	// Mock globals for the tests
	config.osPathSeparator = "/"
	config.hostInfo = make(map[string]EndpointInfo)
	config.hostInfo["validHost"] = EndpointInfo{endpointName: "validHost"}
	config.hostInfo["validHost2"] = EndpointInfo{endpointName: "validHost2"}
	config.ignoreDirectories = []string{"ignoreDir", "ignoreDir2"}
	config.universalDirectory = "UniversalConfs"
	config.allUniversalGroups = map[string][]string{
		"UniversalConfs_Group1": {"host14"},
	}

	tests := []struct {
		path     string
		expected struct {
			skipFile bool
		}
	}{
		{"file.txt", struct {
			skipFile bool
		}{true}},
		{"ignoreDir/file.txt", struct {
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
		t.Run(test.path, func(t *testing.T) {
			skipFile := repoFileIsNotValid(test.path)
			if skipFile != test.expected.skipFile {
				t.Errorf("expected skipFile to be %t, got %t", test.expected.skipFile, skipFile)
			}
		})
	}
}

func TestDetermineFileType(t *testing.T) {
	tests := []struct {
		fileMode string
		expected string
	}{
		{"0100644", "regular"},     // Text file
		{"0120000", "unsupported"}, // Special
		{"0040000", "unsupported"}, // Directory
		{"0160000", "unsupported"}, // Git submodule
		{"0100755", "regular"},     // Executable
		{"0100664", "unsupported"}, // Deprecated
		{"0", "unsupported"},       // Empty (no file)
		{"", "unsupported"},        // Empty string
		{"unknown", "unsupported"}, // Unknown - don't process
	}

	for _, test := range tests {
		t.Run(test.fileMode, func(t *testing.T) {
			result := determineFileType(test.fileMode)
			if result != test.expected {
				t.Errorf("determineFileType(%s) = %s; want %s", test.fileMode, result, test.expected)
			}
		})
	}
}

func TestTranslateLocalPathtoRemotePath(t *testing.T) {
	// Mock windows paths- shouldn't affect tests with unix paths
	config.osPathSeparator = "\\"
	config.repositoryPath = "/home/user/repo"

	tests := []struct {
		localRepoPath    string
		expectedHostDir  string
		expectedFilePath string
	}{
		{"host4/etc/nginx/nginx.conf", "host4", "/etc/nginx/nginx.conf"},
		{"host9/etc/some dir/File Number 1", "host9", "/etc/some dir/File Number 1"},
		{"host/dir/file.txt", "host", "/dir/file.txt"},
		{"host2/dir/subdir/file.txt", "host2", "/dir/subdir/file.txt"},
		{"../../otherdir/dir/targetfile", "", ""},
		{"868_host_region1\\etc\\serv\\file1.conf", "868_host_region1", "/etc/serv/file1.conf"},
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
		t.Run(test.localRepoPath, func(t *testing.T) {
			hostDir, targetFilePath := translateLocalPathtoRemotePath(test.localRepoPath)
			if hostDir != test.expectedHostDir {
				t.Errorf("expected hostDir '%s', got '%s'", test.expectedHostDir, hostDir)
			}
			if targetFilePath != test.expectedFilePath {
				t.Errorf("expected targetFilePath '%s', got '%s'", test.expectedFilePath, targetFilePath)
			}
		})
	}
}

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
				name:        "/etc/rmt",
				fsType:      "regular file",
				owner:       "root",
				group:       "root",
				permissions: 640,
				size:        53,
				linkTarget:  "",
				exists:      true,
			},
			expectError: false,
		},
		{
			name:       "normal file second format",
			statOutput: "[/etc/RMT],[Regular file],[root],[root],[0640],[4853],['/etc/rmt']",
			expected: RemoteFileInfo{
				name:        "/etc/RMT",
				fsType:      "regular file",
				owner:       "root",
				group:       "root",
				permissions: 640,
				size:        4853,
				linkTarget:  "",
				exists:      true,
			},
			expectError: false,
		},
		{
			name:       "normal file bsd",
			statOutput: "[/usr/local/etc/FILE1],[Regular file],[root],[wheel],[644],[254],[target=]",
			expected: RemoteFileInfo{
				name:        "/usr/local/etc/FILE1",
				fsType:      "regular file",
				owner:       "root",
				group:       "wheel",
				permissions: 644,
				size:        254,
				linkTarget:  "",
				exists:      true,
			},
			expectError: false,
		},
		{
			name:       "symbolic link bsd",
			statOutput: "[/etc/File1],[Symbolic Link],[root],[root],[0755],[11],[target=/etc/conf/FILE2]",
			expected: RemoteFileInfo{
				name:        "/etc/File1",
				fsType:      "symbolic link",
				owner:       "root",
				group:       "root",
				permissions: 755,
				size:        11,
				linkTarget:  "/etc/conf/FILE2",
				exists:      true,
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
				name:        "/etc/rmt",
				fsType:      "regular file",
				owner:       "root",
				group:       "root",
				permissions: 777,
				size:        1024,
				linkTarget:  "",
				exists:      true,
			},
			expectError: false,
		},
		{
			name:       "symlink",
			statOutput: "[/etc/r mt],[regular file],[root],[root],[777],[1024],['/etc/r mt' -> '/usr/sbin/rm t']",
			expected: RemoteFileInfo{
				name:        "/etc/r mt",
				fsType:      "regular file",
				owner:       "root",
				group:       "root",
				permissions: 777,
				size:        1024,
				linkTarget:  "/usr/sbin/rm t",
				exists:      true,
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fileInfo, err := extractMetadataFromStat(test.statOutput)

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
			if fileInfo.name != test.expected.name {
				t.Errorf("expected name: '%s', got: '%s'", test.expected.name, fileInfo.name)
			}
			if fileInfo.fsType != test.expected.fsType {
				t.Errorf("expected fsType: '%s', got: '%s'", test.expected.fsType, fileInfo.fsType)
			}
			if fileInfo.owner != test.expected.owner {
				t.Errorf("expected owner: '%s', got: '%s'", test.expected.owner, fileInfo.owner)
			}
			if fileInfo.group != test.expected.group {
				t.Errorf("expected group: '%s', got: '%s'", test.expected.group, fileInfo.group)
			}
			if fileInfo.permissions != test.expected.permissions {
				t.Errorf("expected permissions: '%d', got: '%d'", test.expected.permissions, fileInfo.permissions)
			}
			if fileInfo.size != test.expected.size {
				t.Errorf("expected size: '%d', got: '%d'", test.expected.size, fileInfo.size)
			}
			if fileInfo.linkTarget != test.expected.linkTarget {
				t.Errorf("expected linkTarget: '%s', got: '%s'", test.expected.linkTarget, fileInfo.linkTarget)
			}
			if fileInfo.exists != test.expected.exists {
				t.Errorf("expected exists: '%t', got: '%t'", test.expected.exists, fileInfo.exists)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		// Edge case: 0 bytes
		{0, "0 Bytes"},

		// Small number of bytes
		{500, "500.00 Bytes"},

		// Kilobyte values
		{1024, "1.00 KiB"},
		{2048, "2.00 KiB"},
		{5000, "4.88 KiB"},

		// Megabyte values
		{1048576, "1.00 MiB"},
		{2097152, "2.00 MiB"},
		{5000000, "4.77 MiB"},

		// Gigabyte values
		{1073741824, "1.00 GiB"},
		{2147483648, "2.00 GiB"},
		{5000000000, "4.66 GiB"},

		// Handling the highest unit in the list
		{9223372036854775807, "8192.00 PiB"}, // A very large number

		// Testing the upper bound of the units
		{1099511627776, "1.00 TiB"}, // This should return 1 TiB as the value
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			result := formatBytes(test.input)
			if result != test.expected {
				t.Errorf("For input %d, expected %s but got %s", test.input, test.expected, result)
			}
		})
	}
}

func TestIsText(t *testing.T) {
	tests := []struct {
		name         string
		input        []byte
		expectedText bool
	}{
		{
			name:         "Empty Input",
			input:        []byte{},
			expectedText: true,
		},
		{
			name:         "Single ASCII Byte",
			input:        []byte{84},
			expectedText: true,
		},
		{
			name:         "Single Non-ASCII Byte",
			input:        []byte{250},
			expectedText: false,
		},
		{
			name:         "Regular Text",
			input:        []byte("This is some plain text here but is also some extra data to expand the full data. Also adding some additional characters like ! or even ? or maybe %."),
			expectedText: true,
		},
		{
			name:         "JPG",
			input:        []byte{255, 216, 255, 224, 0, 16, 74, 70, 73, 70, 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 255, 219, 0, 132, 0, 8, 6, 6, 7, 6, 5, 8, 7, 7, 7, 9, 9, 8, 10, 12, 20, 13, 12, 11, 11, 12, 25, 18, 19, 15, 20, 29, 26, 31, 30, 29, 26, 28, 28, 32, 36, 46, 39, 32, 34, 44, 35, 28, 28, 40, 55, 41, 44, 48, 49, 52, 52, 52, 31, 39, 57, 61, 56, 50, 60, 46, 51, 52, 50, 1, 9, 9, 9, 12, 11, 12, 24, 13, 13, 24, 50, 33, 28, 33, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 255, 194, 0, 17, 8, 4, 114, 7, 128, 3, 1, 34, 0, 2, 17, 1, 3, 17, 1, 255, 196, 0, 53, 0, 0, 1, 5, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 0, 2, 3, 4, 6, 1, 7, 8, 1, 0, 3, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 3, 1, 4, 5, 6, 7, 255, 218, 0, 12, 3, 1, 0, 2, 16, 3, 16, 0, 0, 0, 217, 90, 205, 90, 238, 242, 55, 22, 114, 197, 229, 114, 170, 181, 153, 81, 241, 91, 143, 94, 139, 227, 190, 32, 17, 219, 16, 175, 60, 157, 3, 96, 47, 203, 90, 129, 58, 46, 180, 78, 1, 183, 166, 138, 76, 167, 87, 118, 23, 49, 247, 179, 116, 104, 44, 43, 167, 105, 85, 90, 118, 122, 221, 213, 191, 96, 107, 179, 74, 74, 54, 202, 61, 247, 209, 112, 17, 237, 57, 21, 200, 206, 54, 234, 210, 197, 154, 214, 82, 147, 79, 94, 124, 165, 206, 40, 146, 210, 83, 165, 13, 33, 200, 165, 141, 161, 90, 118, 73, 185, 201, 161, 147, 54, 91, 48, 89, 90, 86, 188, 201, 135, 137, 208, 191, 14, 85, 187, 67, 112, 40, 219, 149, 43, 202, 38, 185, 64, 182, 231, 154, 179, 166, 108, 79, 154, 60, 222, 68, 248, 66, 24, 186, 198, 200, 171, 216, 135, 114, 20, 235, 27, 157, 176, 150, 107, 32, 81, 7, 26, 150, 143, 123, 28, 18, 246, 23, 3, 163, 76, 14, 181, 177, 105, 51, 169, 243, 114, 243, 199, 119, 2, 86, 194, 144, 205, 208, 92, 5, 110, 110, 66, 200, 217, 87, 72, 114, 148, 202, 215, 172, 85, 177, 140, 238, 177, 152, 210, 207, 66, 208, 75, 214, 37, 210, 86, 168, 217, 90, 223, 176, 58, 117, 173, 229, 81, 102, 217, 237, 110, 155, 97, 213, 86, 18, 199, 3, 53, 105, 10, 58, 34, 144, 8, 32, 168, 59, 242, 141, 166, 250, 53, 155, 105, 77, 89, 241, 239, 134, 109, 88, 96, 179, 221, 106, 44, 159, 172, 114, 213, 137, 147, 33, 183, 86, 214, 5, 13, 15, 191, 26, 16, 38, 46, 220, 170, 86, 106, 183, 18, 180, 199, 159, 136, 49, 109, 43, 90, 208, 28, 203, 213, 117, 106, 71, 106, 201, 131, 228, 49, 56, 193, 89, 161, 234, 232, 107, 134, 58, 48, 120, 47, 244, 193, 113, 28, 31, 170, 58, 50, 21, 117, 99, 236, 150, 52, 225, 56, 175, 206, 183, 19, 98, 199, 146, 140, 177, 10},
			expectedText: false,
		},
		{
			name:         "PNG",
			input:        []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 2, 0, 0, 0, 2, 0, 8, 6, 0, 0, 0, 244, 120, 212, 250, 0, 0, 34, 145, 73, 68, 65, 84, 120, 218, 236, 221, 177, 13, 195, 48, 16, 4, 65, 39, 86, 93, 10, 216, 127, 77, 78, 92, 192, 1, 78, 228, 197, 124, 1, 67, 108, 116, 33, 95, 231, 220, 215, 57, 247, 251, 245, 195, 157, 115, 191, 191, 206, 197, 227, 241, 120, 60, 30, 239, 15, 188, 84, 12, 143, 199, 227, 241, 120, 188, 237, 202, 49, 60, 30, 143, 199, 227, 241, 140, 63, 143, 199, 227, 241, 120, 188, 114, 12, 143, 199, 227, 241, 120, 60, 227, 207, 227, 241, 120, 60, 30, 175, 28, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 111, 243, 158, 249, 56, 143, 199, 227, 241, 120, 60, 227, 207, 227, 241, 120, 60, 30, 207, 248, 243, 120, 60, 30, 143, 199, 51, 254, 60, 30, 143, 199, 227, 241, 140, 63, 143, 199, 227, 241, 120, 60, 227, 207, 227, 241, 120, 60, 30, 175, 30, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 182, 75, 197, 240, 120, 60, 30, 143, 199, 219, 174, 28, 195, 227, 241, 120, 60, 30, 207, 248, 243, 120, 60, 30, 143, 199, 43, 199, 240, 120, 60, 30, 143, 199, 51, 254, 60, 30, 143, 199, 227, 241, 202, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 252, 18, 200, 227, 241, 120, 60, 30, 207, 248, 243, 120, 60, 30, 143, 199, 123, 230, 227, 60, 30, 143, 199, 227, 241, 140, 63, 143, 199, 227, 241, 120, 60, 227, 207, 227, 241, 120, 60, 30, 207, 248, 243, 120, 60, 30, 143, 199, 51, 254, 60, 30, 143, 199, 227, 241, 118, 47, 21, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 111, 187, 84, 12, 143, 199, 227, 241, 120, 188, 237, 202, 49, 60, 30, 143, 199, 227, 241, 140, 63, 143, 199, 227, 241, 120, 188, 114, 12, 143, 199, 227, 241, 120, 60, 227, 207, 227, 241, 120, 60, 30, 175, 28, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 111, 243, 82, 49, 60, 30, 143, 199, 227, 241, 54, 47, 21, 195, 227, 241, 120, 60, 30, 207, 47, 129, 60, 30, 143, 199, 227, 241, 140, 63, 143, 199, 227, 241, 120, 188, 103, 62, 206, 227, 241},
			expectedText: false,
		},
		{
			name:         "PDF",
			input:        []byte{37, 80, 68, 70, 45, 49, 46, 52, 10, 37, 211, 235, 233, 225, 10, 49, 32, 48, 32, 111, 98, 106, 10, 60, 60, 47, 84, 105, 116, 108, 101, 32, 40, 66, 114, 111, 99, 104, 117, 114, 101, 41, 10, 47, 80, 114, 111, 100, 117, 99, 101, 114, 32, 40, 83, 107, 105, 97, 47, 80, 68, 70, 32, 109, 49, 49, 49, 32, 71, 111, 111, 103, 108, 101, 32, 68, 111, 99, 115, 32, 82, 101, 110, 100, 101, 114, 101, 114, 41, 62, 62, 10, 101, 110, 100, 111, 98, 106, 10, 51, 32, 48, 32, 111, 98, 106, 10, 60, 60, 47, 99, 97, 32, 49, 10, 47, 66, 77, 32, 47, 78, 111, 114, 109, 97, 108, 62, 62, 10, 101, 110, 100, 111, 98, 106, 10, 54, 32, 48, 32, 111, 98, 106, 10, 60, 60, 47, 84, 121, 112, 101, 32, 47, 88, 79, 98, 106, 101, 99, 116, 10, 47, 83, 117, 98, 116, 121, 112, 101, 32, 47, 73, 109, 97, 103, 101, 10, 47, 87, 105, 100, 116, 104, 32, 56, 48, 48, 10, 47, 72, 101, 105, 103, 104, 116, 32, 56, 48, 48, 10, 47, 67, 111, 108, 111, 114, 83, 112, 97, 99, 101, 32, 47, 68, 101, 118, 105, 99, 101, 82, 71, 66, 10, 47, 83, 77, 97, 115, 107, 32, 55, 32, 48, 32, 82, 10, 47, 66, 105, 116, 115, 80, 101, 114, 67, 111, 109, 112, 111, 110, 101, 110, 116, 32, 56, 10, 47, 70, 105, 108, 116, 101, 114, 32, 47, 70, 108, 97, 116, 101, 68, 101, 99, 111, 100, 101, 10, 47, 76, 101, 110, 103, 116, 104, 32, 57, 57, 56, 55, 62, 62, 32, 115, 116, 114, 101, 97, 109, 10, 120, 156, 237, 221, 189, 178, 37, 197, 181, 133, 209, 126, 255, 71, 187, 10, 60, 57, 152, 120, 114, 192, 130, 80, 224, 221, 134, 70, 77, 115, 250, 252, 236, 159, 154, 53, 51, 87, 141, 17, 60, 192, 217, 85, 43, 106, 125, 36, 169, 208, 127, 255, 251, 223, 255, 252, 231, 63, 191, 3, 0, 112, 144, 223, 126, 251, 237, 191, 18, 139, 67, 253, 10, 1, 63, 67, 70, 123, 180, 153, 233, 183, 63, 73, 44, 14, 212, 254, 88, 50, 211, 79, 144, 209, 30, 109, 102, 250, 237, 127, 36, 22, 71, 249, 213, 81, 3, 1, 159, 63, 80, 237, 61, 204, 76, 237, 209, 102, 166, 79, 159, 62, 73, 44, 142, 165, 175, 72, 208, 87, 132, 180, 71, 155, 153, 62, 253, 73, 98, 113, 32, 125, 69, 130, 190, 34, 164, 61, 218, 204, 244, 233, 127, 36, 22, 71, 209, 87, 36, 232, 43, 66, 218, 163, 205, 76, 159, 190, 33, 177, 56, 132, 190, 34, 65, 95, 17, 210, 30, 109, 102, 250, 244, 79, 18, 139, 231, 233, 43, 18, 244, 21, 33, 237, 209, 102, 166, 79, 223, 145, 88, 60, 73, 95, 145, 160, 175, 8, 105, 143, 54, 51, 125, 223, 87, 18, 139, 39, 233, 43, 18, 244, 21, 33, 237, 209, 102, 166, 87, 251, 74, 98, 241, 12, 125, 69, 130, 190, 34, 164, 61, 218, 204, 244, 86, 95, 73, 44, 30, 166, 175, 72, 208, 87, 132, 180, 71, 155, 153, 222, 233, 43, 137, 197, 99, 244, 21, 9, 250, 138, 144, 246, 104, 51, 211, 251, 125, 37, 177, 120, 128, 190, 34, 65, 95, 17, 210, 30, 109, 102, 250, 176, 175, 36, 22, 247, 210, 87, 36},
			expectedText: false,
		},
		{
			name:         "ELF Binary",
			input:        []byte{127, 69, 76, 70, 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 62, 0, 1, 0, 0, 0, 96, 126, 71, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 88, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 56, 0, 5, 0, 64, 0, 14, 0, 13, 0, 6, 0, 0, 0, 4, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 64, 0, 64, 0, 0, 0, 0, 0, 64, 0, 64, 0, 0, 0, 0, 0, 24, 1, 0, 0, 0, 0, 0, 0, 24, 1, 0, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 49, 33, 60, 0, 0, 0, 0, 0, 49, 33, 60, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 4, 0, 0, 0, 0, 48, 60, 0, 0, 0, 0, 0, 0, 48, 124, 0, 0, 0, 0, 0, 0, 48, 124, 0, 0, 0, 0, 0, 104, 26, 62, 0, 0, 0, 0, 0, 104, 26, 62, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 80, 122, 0, 0, 0, 0, 0, 0, 80, 186, 0, 0, 0, 0, 0, 0, 80, 186, 0, 0, 0, 0, 0, 224, 229, 5, 0, 0, 0, 0, 0, 80, 253, 8, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 81, 229, 116, 100, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 16, 64, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 49, 17, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 89, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 48, 124, 0, 0, 0, 0, 0, 0, 48, 60, 0, 0, 0, 0, 0, 129, 213, 24, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 97, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 160, 5, 149, 0, 0, 0, 0, 0, 160, 5, 85, 0, 0, 0, 0, 0, 96, 41, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 107, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 47, 149, 0, 0, 0, 0, 0, 0, 47, 85, 0, 0, 0, 0, 0, 120, 12, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 117, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 120, 59, 149, 0, 0, 0, 0, 0, 120, 59, 85, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 127, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 128, 59},
			expectedText: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stringIsText := isText(&test.input)

			if stringIsText != test.expectedText {
				t.Errorf("Expected input to be text? %t - Text is actually text? %t", test.expectedText, stringIsText)
			}
		})
	}
}
