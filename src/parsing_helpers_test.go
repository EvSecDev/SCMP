// controller
package main

import (
	"fmt"
	"testing"
)

// Unit test for checkForOverride
func TestCheckForOverride(t *testing.T) {
	// Mock globals
	globalVerbosityLevel = 0
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
	config.universalDirectory = "Universal"
	config.hostInfo = map[string]EndpointInfo{
		"868_host_region1": {},
		"host":             {},
		"host1":            {},
		"host2":            {},
		"host3":            {},
		"host4":            {},
		"host9":            {},
		"!@#$%^&*()_+":     {},
	}
	config.allUniversalGroups = map[string][]string{
		"Universal_VMs": {"host4", "host9"},
	}

	tests := []struct {
		localRepoPath    string
		expectedHostDir  string
		expectedFilePath string
	}{
		{"host4/etc/nginx/nginx.conf", "host4", "/etc/nginx/nginx.conf"},
		{"host9/etc/some dir/File Number 1", "host9", "/etc/some dir/File Number 1"},
		{"host/dir/file.txt", "host", "/dir/file.txt"},
		{"host2/dir/subdir/file.txt", "host2", "/dir/subdir/file.txt"},
		{"Universal/etc/resolv.conf", "Universal", "/etc/resolv.conf"},
		{"Universal_VMs/etc/modules.d/01load", "Universal_VMs", "/etc/modules.d/01load"},
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
