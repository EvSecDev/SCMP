// controller
package main

import (
	"fmt"
	"sort"
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
		config.regexEnabled = test.useRegex

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
			skipFile := repoFileIsValid(test.path)
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
		{"0120000", "symlink"},     // Special, but able to be handled
		{"0040000", "unsupported"}, // Directory
		{"0160000", "unsupported"}, // Git submodule
		{"0100755", "unsupported"}, // Executable
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

	tests := []struct {
		localRepoPath    string
		expectedHostDir  string
		expectedFilePath string
	}{
		{"host/dir/file.txt", "host", "/dir/file.txt"},
		{"host2/dir/subdir/file.txt", "host2", "/dir/subdir/file.txt"},
		{"868_host_region1\\etc\\serv\\file1.conf", "868_host_region1", "/etc/serv/file1.conf"},
		{"file1.txt", "", ""},
		{"", "", ""},
		{"host3/dir/pic.jpeg.remote-artifact", "host3", "/dir/pic.jpeg"},
		{"/home/user/repo/host1/file", "", "/home/user/repo/host1/file"},
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

func TestPermissionsSymbolicToNumeric(t *testing.T) {
	// Define test cases
	tests := []struct {
		input       string
		expected    int
		expectedErr bool
	}{
		{"rwxr-xr-x", 755, false},      // Full permissions for owner, read and execute for others
		{"rw-r--r--", 644, false},      // Read/write for owner, read-only for others
		{"r--r--r--", 444, false},      // Read-only for everyone
		{"rw-rw-rw-", 666, false},      // Read and write for everyone
		{"rwx------", 700, false},      // Full permissions for owner only
		{"------x", 1, false},          // Only execute permission for others
		{"", 0, true},                  // No input
		{"text", 0, true},              // Too short/wrong input
		{"thistextistoolong", 0, true}, // Too long text input
	}

	// Iterate over test cases
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			// Call the function
			result, err := permissionsSymbolicToNumeric(test.input)

			// Check if the result matches the expected value
			if (err != nil) != test.expectedErr {
				t.Errorf("For input %s, error = %v, wantErr %v", test.input, err, test.expectedErr)
			}
			if result != test.expected {
				t.Errorf("For input %s, expected %d, but got %d", test.input, test.expected, result)
			}
		})
	}
}

func TestExtractMetadataFromLS(t *testing.T) {
	tests := []struct {
		name               string
		lsOutput           string
		expectedType       string
		expectedPerms      int
		expectedOwner      string
		expectedGroup      string
		expectedSize       int
		expectedName       string
		expectedLinkTarget string
		expectedErr        bool
	}{
		{
			name:               "Valid input",
			lsOutput:           "-rwxr-xr-x 1 user group 1234 Jan 1 12:34 filename",
			expectedType:       "-",
			expectedPerms:      755,
			expectedOwner:      "user",
			expectedGroup:      "group",
			expectedSize:       1234,
			expectedName:       "filename",
			expectedLinkTarget: "",
			expectedErr:        false,
		},
		{
			name:               "Incomplete input",
			lsOutput:           "drwxr-xr-x",
			expectedType:       "",
			expectedPerms:      0,
			expectedOwner:      "",
			expectedGroup:      "",
			expectedSize:       0,
			expectedName:       "",
			expectedLinkTarget: "",
			expectedErr:        true,
		},
		{
			name:               "Invalid size",
			lsOutput:           "-rwxr-x--- 1 user group invalid_size Jan 1 12:34 filename",
			expectedType:       "-",
			expectedPerms:      750,
			expectedOwner:      "user",
			expectedGroup:      "group",
			expectedSize:       0,
			expectedName:       "filename",
			expectedLinkTarget: "",
			expectedErr:        true,
		},
		{
			name:               "One-Too-Short",
			lsOutput:           "-rwxr-x-w- 1 user group 123 Jan 12:34 /etc/file",
			expectedType:       "",
			expectedPerms:      0,
			expectedOwner:      "",
			expectedGroup:      "",
			expectedSize:       0,
			expectedName:       "",
			expectedLinkTarget: "",
			expectedErr:        true,
		},
		{
			name:               "Symbolic Link",
			lsOutput:           "lrwxrwxrwx 1 root root 13 Jan  1 2024 /opt/exe -> /usr/bin/exe",
			expectedType:       "l",
			expectedPerms:      777,
			expectedOwner:      "root",
			expectedGroup:      "root",
			expectedSize:       13,
			expectedName:       "/opt/exe",
			expectedLinkTarget: "/usr/bin/exe",
			expectedErr:        false,
		},
		{
			name:               "Device File",
			lsOutput:           "crw--w---- 1 root tty 4, 0 Jan 12 01:23 /dev/tty0",
			expectedType:       "c",
			expectedPerms:      620,
			expectedOwner:      "root",
			expectedGroup:      "tty",
			expectedSize:       0,
			expectedName:       "01:23",
			expectedLinkTarget: "",
			expectedErr:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := extractMetadataFromLS(test.lsOutput)

			if (err != nil) != test.expectedErr {
				t.Errorf("extractMetadataFromLS() error = %v, wantErr %v", err, test.expectedErr)
			}
			if metadata.fsType != test.expectedType {
				t.Errorf("extractMetadataFromLS() Type = %v, want %v", metadata.fsType, test.expectedType)
			}
			if metadata.permissions != test.expectedPerms {
				t.Errorf("extractMetadataFromLS() Permissions = %v, want %v", metadata.permissions, test.expectedPerms)
			}
			if metadata.owner != test.expectedOwner {
				t.Errorf("extractMetadataFromLS() Owner = %v, want %v", metadata.owner, test.expectedOwner)
			}
			if metadata.group != test.expectedGroup {
				t.Errorf("extractMetadataFromLS() Group = %v, want %v", metadata.group, test.expectedGroup)
			}
			if metadata.size != test.expectedSize {
				t.Errorf("extractMetadataFromLS() Size = %v, want %v", metadata.size, test.expectedSize)
			}
			if metadata.linkTarget != test.expectedLinkTarget {
				t.Errorf("extractMetadataFromLS() Link = %v, want %v", metadata.linkTarget, test.expectedLinkTarget)
			}
			if metadata.name != test.expectedName {
				t.Errorf("extractMetadataFromLS() Name = %v, want %v", metadata.name, test.expectedName)
			}
		})
	}
}

func TestCheckForDiff(t *testing.T) {
	tests := []struct {
		name                    string
		remoteMetadata          RemoteFileInfo
		localMetadata           FileInfo
		expectedContentDiffers  bool
		expectedMetadataDiffers bool
	}{
		{
			name: "Everything differs",
			remoteMetadata: RemoteFileInfo{
				hash:        "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
				permissions: 757,
				owner:       "user1",
				group:       "group1",
			},
			localMetadata: FileInfo{
				hash:        "590c9f8430c7435807df8ba9a476e3f1295d46ef210f6efae2043a4c085a569e",
				permissions: 640,
				ownerGroup:  "user2:group1",
			},
			expectedContentDiffers:  true,
			expectedMetadataDiffers: true,
		},
		{
			name: "Hashes differ",
			remoteMetadata: RemoteFileInfo{
				hash: "1b4f0e9851971998e732078544c96b36c3d01cedf7caa332359d6f1d83567014",
			},
			localMetadata: FileInfo{
				hash: "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
			},
			expectedContentDiffers:  true,
			expectedMetadataDiffers: false,
		},
		{
			name: "Permissions differ",
			remoteMetadata: RemoteFileInfo{
				permissions: 757,
			},
			localMetadata: FileInfo{
				permissions: 640,
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: true,
		},
		{
			name: "Owner and group differ",
			remoteMetadata: RemoteFileInfo{
				owner: "user1",
				group: "group1",
			},
			localMetadata: FileInfo{
				ownerGroup: "user2:group2",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: true,
		},
		{
			name: "No differences",
			remoteMetadata: RemoteFileInfo{
				hash:        "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
				permissions: 0755,
				owner:       "user1",
				group:       "group1",
			},
			localMetadata: FileInfo{
				hash:        "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752",
				permissions: 0755,
				ownerGroup:  "user1:group1",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: false,
		},
		{
			name: "No data",
			remoteMetadata: RemoteFileInfo{
				hash:        "",
				permissions: 0,
				owner:       "",
				group:       "",
			},
			localMetadata: FileInfo{
				hash:        "",
				permissions: 0,
				ownerGroup:  "",
			},
			expectedContentDiffers:  false,
			expectedMetadataDiffers: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contentDiffers, metadataDiffers := checkForDiff(test.remoteMetadata, test.localMetadata)

			if contentDiffers != test.expectedContentDiffers {
				t.Errorf("expected contentDiffers %v, got %v", test.expectedContentDiffers, contentDiffers)
			}
			if metadataDiffers != test.expectedMetadataDiffers {
				t.Errorf("expected metadataDiffers %v, got %v", test.expectedMetadataDiffers, metadataDiffers)
			}
		})
	}
}

// Helper function to compare slices of strings
func compareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Helper function to compare maps of string to slice of strings
func compareStringMapSlices(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if _, exists := b[key]; !exists {
			return false
		}
		if !compareStringSlices(value, b[key]) {
			return false
		}
	}
	return true
}

// Test function for groupFilesByReloads
func TestGroupFilesByReloads(t *testing.T) {
	tests := []struct {
		name              string
		allFileInfo       map[string]FileInfo
		repoFilePaths     []string
		expectedByCommand map[string][]string
		expectedNoReload  []string
	}{
		{
			name: "files with reload and no reload",
			allFileInfo: map[string]FileInfo{
				"file1": {reloadRequired: true, reload: []string{"cmd50", "cmd51", "cmd52"}},
				"file2": {reloadRequired: true, reload: []string{"cmd40", "cmd41"}},
				"file3": {reloadRequired: false, reload: nil},
			},
			repoFilePaths: []string{"file1", "file2", "file3"},
			expectedByCommand: map[string][]string{
				"W2NtZDUwIGNtZDUxIGNtZDUyXQ==": {"file1"},
				"W2NtZDQwIGNtZDQxXQ==":         {"file2"},
			},
			expectedNoReload: []string{"file3"},
		},
		{
			name: "all files with the same reload command",
			allFileInfo: map[string]FileInfo{
				"file1": {reloadRequired: true, reload: []string{"cmd30", "cmd32", "cmd^$"}},
				"file2": {reloadRequired: true, reload: []string{"cmd30", "cmd32", "cmd^$"}},
				"file3": {reloadRequired: false, reload: nil},
			},
			repoFilePaths: []string{"file1", "file2", "file3"},
			expectedByCommand: map[string][]string{
				"W2NtZDMwIGNtZDMyIGNtZF4kXQ==": {"file1", "file2"},
			},
			expectedNoReload: []string{"file3"},
		},
		{
			name: "no files with reload commands",
			allFileInfo: map[string]FileInfo{
				"file1": {reloadRequired: false, reload: nil},
				"file2": {reloadRequired: false, reload: nil},
			},
			repoFilePaths:     []string{"file1", "file2"},
			expectedByCommand: map[string][]string{}, // No files with reloads
			expectedNoReload:  []string{"file1", "file2"},
		},
		{
			name:              "empty input",
			allFileInfo:       map[string]FileInfo{},
			repoFilePaths:     []string{},
			expectedByCommand: map[string][]string{}, // No files
			expectedNoReload:  []string{},            // No files
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function being tested
			commitFileByCommand, commitFilesNoReload := groupFilesByReloads(tt.allFileInfo, tt.repoFilePaths)

			// Check if the result matches the expected output for commitFileByCommand
			if !compareStringMapSlices(commitFileByCommand, tt.expectedByCommand) {
				t.Errorf("expected commitFileByCommand: %v, got: %v", tt.expectedByCommand, commitFileByCommand)
			}

			// Check if the result matches the expected output for commitFilesNoReload
			if !compareStringSlices(commitFilesNoReload, tt.expectedNoReload) {
				t.Errorf("expected commitFilesNoReload: %v, got: %v", tt.expectedNoReload, commitFilesNoReload)
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
