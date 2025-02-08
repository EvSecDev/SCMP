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
		AllUniversalGroups: map[string]struct{}{
			"universalGroup1": {},
			"universalGroup2": {},
		},
		HostInfo: map[string]EndpointInfo{
			"host1": {
				UniversalGroups: map[string]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host2": {
				UniversalGroups: map[string]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host3": {
				UniversalGroups: map[string]struct{}{
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
	}{
		{"", "host1", false},
		{"host1", "host1", false},
		{"host1,host2", "host1", false},
		{"host1,host2", "host3", true},
		{"host1, host2", "host3", true},
		{"host1, host2, host3, host4, host5, host6", "host3", true},
		{"file1.txt,file2.txt", "file1.txt", false},
		{"file1.txt,file2.txt", "file3.txt", true},
		{"file!@%$^&*(4.txt,file6.txt", "file6.txt", false},
		{"file!@%$^&*(4.txt,file6.txt", "file!@%$^&*(4.txt", false},
		{"universalconfs/*", "universalconfs/etc/hosts", false},
		{"universalconfs/etc/*", "universalconfs/var/log/file.txt", true},
		{"universalconfs/*", "universalconfs_ssh/etc/ssh/sshd_config", true},
		{"host0*", "host0436", false},
		{"UniversalConfs_Service1", "host2", false},
		{"UniversalConfs_Service1", "host3", true},
	}

	for _, test := range tests {
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
		OSPathSeparator:    "/",
		UniversalDirectory: "universal",
		AllUniversalGroups: map[string]struct{}{
			"universalGroup1": {},
			"universalGroup2": {},
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
		HostInfo: map[string]EndpointInfo{
			"host1": {
				UniversalGroups: map[string]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host2": {
				UniversalGroups: map[string]struct{}{
					"UniversalConfs_OtherServers": {},
				},
			},
			"host3": {
				UniversalGroups: map[string]struct{}{
					"": {},
				},
			},
		},
		UniversalDirectory: "UniversalConfs",
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
				if remaining != test.expectedRemainingContent {
					t.Errorf("expected remaining content '%v', got '%v'", test.expectedRemainingContent, remaining)
				}
			}
		})
	}
}

func TestValidateRepoFile(t *testing.T) {
	// Mock globals for the tests
	config.OSPathSeparator = "/"
	config.HostInfo = make(map[string]EndpointInfo)
	config.HostInfo["validHost"] = EndpointInfo{EndpointName: "validHost"}
	config.HostInfo["validHost2"] = EndpointInfo{EndpointName: "validHost2"}
	config.IgnoreDirectories = []string{"ignoreDir", "ignoreDir2"}
	config.UniversalDirectory = "UniversalConfs"
	config.AllUniversalGroups = map[string]struct{}{
		"UniversalConfs_Group1": {},
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

func TestSeparateHostDirFromPath(t *testing.T) {
	tests := []struct {
		localRepoPath    string
		expectedHostDir  string
		expectedFilePath string
	}{
		{"host/dir/file.txt", "host", "/dir/file.txt"},
		{"host2/dir/subdir/file.txt", "host2", "/dir/subdir/file.txt"},
		{"file1.txt", "", ""},
		{"", "", ""},
		{"/home/user/repo/host1/file", "", "/home/user/repo/host1/file"},
		{"!@#$%^&*()_+/etc/file", "!@#$%^&*()_+", "/etc/file"},
	}

	for _, test := range tests {
		t.Run(test.localRepoPath, func(t *testing.T) {
			hostDir, targetFilePath := separateHostDirFromPath(test.localRepoPath)
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
		input    string
		expected int
	}{
		{"rwxr-xr-x", 755}, // Full permissions for owner, read and execute for others
		{"rw-r--r--", 644}, // Read/write for owner, read-only for others
		{"r--r--r--", 444}, // Read-only for everyone
		{"rw-rw-rw-", 666}, // Read and write for everyone
		{"rwx------", 700}, // Full permissions for owner only
		{"------x", 1},     // Only execute permission for others
	}

	// Iterate over test cases
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			// Call the function
			result := permissionsSymbolicToNumeric(test.input)

			// Check if the result matches the expected value
			if result != test.expected {
				t.Errorf("For input %s, expected %d, but got %d", test.input, test.expected, result)
			}
		})
	}
}

func TestExtractMetadataFromLS(t *testing.T) {
	tests := []struct {
		name          string
		lsOutput      string
		expectedType  string
		expectedPerms string
		expectedOwner string
		expectedGroup string
		expectedSize  int
		expectedName  string
		expectedErr   bool
	}{
		{
			name:          "Valid input",
			lsOutput:      "-rwxr-xr-x 1 user group 1234 Jan 1 12:34 filename",
			expectedType:  "-",
			expectedPerms: "rwxr-xr-x",
			expectedOwner: "user",
			expectedGroup: "group",
			expectedSize:  1234,
			expectedName:  "filename",
			expectedErr:   false,
		},
		{
			name:          "Incomplete input",
			lsOutput:      "drwxr-xr-x",
			expectedType:  "",
			expectedPerms: "",
			expectedOwner: "",
			expectedGroup: "",
			expectedSize:  0,
			expectedName:  "",
			expectedErr:   true,
		},
		{
			name:          "Invalid size",
			lsOutput:      "-rwxr-xr-x 1 user group invalid_size Jan 1 12:34 filename",
			expectedType:  "-",
			expectedPerms: "rwxr-xr-x",
			expectedOwner: "user",
			expectedGroup: "group",
			expectedSize:  0,
			expectedName:  "filename",
			expectedErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			Type, Permissions, Owner, Group, Size, Name, err := extractMetadataFromLS(test.lsOutput)

			if (err != nil) != test.expectedErr {
				t.Errorf("extractMetadataFromLS() error = %v, wantErr %v", err, test.expectedErr)
			}
			if Type != test.expectedType {
				t.Errorf("extractMetadataFromLS() Type = %v, want %v", Type, test.expectedType)
			}
			if Permissions != test.expectedPerms {
				t.Errorf("extractMetadataFromLS() Permissions = %v, want %v", Permissions, test.expectedPerms)
			}
			if Owner != test.expectedOwner {
				t.Errorf("extractMetadataFromLS() Owner = %v, want %v", Owner, test.expectedOwner)
			}
			if Group != test.expectedGroup {
				t.Errorf("extractMetadataFromLS() Group = %v, want %v", Group, test.expectedGroup)
			}
			if Size != test.expectedSize {
				t.Errorf("extractMetadataFromLS() Size = %v, want %v", Size, test.expectedSize)
			}
			if Name != test.expectedName {
				t.Errorf("extractMetadataFromLS() Name = %v, want %v", Name, test.expectedName)
			}
		})
	}
}
