// controller
package main

import (
	"fmt"
	"sort"
	"testing"
)

func TestParseChangedFiles(t *testing.T) {
	// Mock Globals
	globalVerbosityLevel = 0
	config.osPathSeparator = "/"
	config.hostInfo = map[string]EndpointInfo{
		"host1": {},
		"host2": {},
		"host3": {},
		"host4": {},
	}

	type TestCase struct {
		name                string
		changedFiles        []GitChangedFileMetadata
		fileOverride        string
		expectedCommitFiles map[string]string
	}
	testCases := []TestCase{
		{
			name: "Single - New File",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "",
					fromMode:    "",
					toNotOnFS:   false,
					toPath:      "host1/etc/network/interfaces",
					toMode:      "0100644",
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[string]string{
				"host1/etc/network/interfaces": "create",
			},
		},
		{
			name: "Single - New Dir Meta",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "",
					fromMode:    "",
					toNotOnFS:   false,
					toPath:      "host1/var/www/site/" + directoryMetadataFileName,
					toMode:      "0100644",
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[string]string{
				"host1/var/www/site/" + directoryMetadataFileName: "dirCreate",
			},
		},
		{
			name: "Single - Modified Dir Meta",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host2/opt/prog/" + directoryMetadataFileName,
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host2/opt/prog/" + directoryMetadataFileName,
					toMode:      "0100644",
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[string]string{
				"host2/opt/prog/" + directoryMetadataFileName: "dirModify",
			},
		},
		{
			name: "Single - Moved to another host",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "host1/etc/network/interfaces",
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host2/etc/network/interfaces",
					toMode:      "0100644",
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[string]string{
				"host2/etc/network/interfaces": "create",
			},
		},
		{
			name: "Multiple - User override",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host2/etc/hostname",
					fromMode:    "0100644",
					toNotOnFS:   true,
					toPath:      "host2/etc/hostname",
					toMode:      "0100644",
				},
				{
					fromNotOnFS: false,
					fromPath:    "host3/etc/resolv.conf",
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host3/etc/resolv.conf",
					toMode:      "0100644",
				},
				{
					fromNotOnFS: false,
					fromPath:    "host4/etc/rsyslog.conf",
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host4/etc/rsyslog.conf",
					toMode:      "0100644",
				},
			},
			fileOverride: "host3/etc/resolv.conf",
			expectedCommitFiles: map[string]string{
				"host3/etc/resolv.conf": "create",
			},
		},
		{
			name: "Single - Same Name",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host1/etc/hosts",
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host1/etc/hosts",
					toMode:      "0100644",
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[string]string{
				"host1/etc/hosts": "create",
			},
		},
		{
			name: "Single - Copied to Other Host",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host1/etc/default/grub",
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host3/etc/default/grub",
					toMode:      "0100644",
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[string]string{
				"host3/etc/default/grub": "create",
			},
		},
		{
			name: "Dual - Rename and In-Place",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: true,
					fromPath:    "host1/etc/hosts",
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host1/etc/backup.hosts",
					toMode:      "0100644",
				},
				{
					fromNotOnFS: false,
					fromPath:    "host2/etc/conf1",
					fromMode:    "0100644",
					toNotOnFS:   false,
					toPath:      "host2/etc/conf1",
					toMode:      "0100644",
				},
			},
			fileOverride: "",
			expectedCommitFiles: map[string]string{
				"host1/etc/backup.hosts": "create",
				"host2/etc/conf1":        "create",
			},
		},
		{
			name: "Modified Unsupported File Type",
			changedFiles: []GitChangedFileMetadata{
				{
					fromNotOnFS: false,
					fromPath:    "host4/dev/sda",
					fromMode:    "0100664",
					toNotOnFS:   false,
					toPath:      "host4/dev/sda",
					toMode:      "0100664",
				},
			},
			fileOverride:        "",
			expectedCommitFiles: map[string]string{},
		},
		{
			name:                "No input",
			changedFiles:        []GitChangedFileMetadata{},
			fileOverride:        "",
			expectedCommitFiles: map[string]string{},
		},
		{
			name:                "Only override input",
			changedFiles:        []GitChangedFileMetadata{},
			fileOverride:        "host1/etc/file.conf,host2/etc/conf.file",
			expectedCommitFiles: map[string]string{},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			commitFiles := parseChangedFiles(test.changedFiles, test.fileOverride)

			if fmt.Sprintf("%v", test.expectedCommitFiles) != fmt.Sprintf("%v", commitFiles) {
				t.Errorf("Expected metadata does not match output metadata:\nOutput:\n%v\n\nExpected Output:\n%v\n", commitFiles, test.expectedCommitFiles)
			}
		})
	}
}

func TestFilterHostsAndFiles(t *testing.T) {
	// Lower verbosity for standard prints
	globalVerbosityLevel = 0

	// Mock global vars
	config = Config{
		osPathSeparator: "/",
		hostInfo: map[string]EndpointInfo{
			"host1": {
				deploymentState: "online",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs_Service1": {}, "UniversalConfs": {}},
				endpointName:    "host1",
			},
			"host2": {
				deploymentState: "",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs_Service2": {}, "UniversalConfs": {}},
				endpointName:    "host2",
			},
			"host3": {
				deploymentState: "go",
				ignoreUniversal: true,
				universalGroups: map[string]struct{}{"": {}},
				endpointName:    "host3",
			},
			"host4": {
				deploymentState: "",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs": {}},
				endpointName:    "host4",
			},
			"host5": {
				deploymentState: "offline",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs": {}},
				endpointName:    "host5",
			},
		},
		universalDirectory: "UniversalConfs",
		allUniversalGroups: map[string][]string{"UniversalConfs_Service1": {"host"}},
		options:            Opts{ignoreDeploymentState: false},
	}

	// Test cases
	type TestCase struct {
		name                 string
		commitFiles          map[string]string
		deniedUniversalFiles map[string]map[string]struct{}
		hostOverride         string
		expectedHosts        []string
		expectedFiles        map[string]string
		expectedFilesByHost  map[string][]string
	}
	testCases := []TestCase{
		{
			name: "Standard Deployment Only Host Files",
			commitFiles: map[string]string{
				"host1/etc/resolv.conf":      "create",
				"host1/etc/hosts":            "create",
				"host2/etc/nginx/nginx.conf": "create",
			},
			expectedHosts: []string{"host1", "host2"},
			expectedFiles: map[string]string{
				"host1/etc/resolv.conf":      "create",
				"host1/etc/hosts":            "create",
				"host2/etc/nginx/nginx.conf": "create",
			},
			expectedFilesByHost: map[string][]string{
				"host1": {"host1/etc/resolv.conf", "host1/etc/hosts"},
				"host2": {"host2/etc/nginx/nginx.conf"},
			},
		},
		{
			name: "Host Override Single Host",
			commitFiles: map[string]string{
				"host1/etc/hosts":              "create",
				"host2/etc/network/interfaces": "create",
				"host3/etc/rsyslog.conf":       "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{
				"host1": {
					"UniversalConfs/etc/some-file": {},
				},
			},
			hostOverride:  "host3",
			expectedHosts: []string{"host3"},
			expectedFiles: map[string]string{
				"host3/etc/rsyslog.conf": "create",
			},
			expectedFilesByHost: map[string][]string{
				"host3": {"host3/etc/rsyslog.conf"},
			},
		},
		{
			name: "Host Ignores Universal",
			commitFiles: map[string]string{
				"UniversalConfs/etc/resolv.conf": "create",
				"host3/etc/hosts":                "create",
				"host3/etc/crontab":              "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{
				"host3": {
					"UniversalConfs/etc/hosts": {},
				},
			},
			hostOverride:  "",
			expectedHosts: []string{"host1", "host2", "host3", "host4"},
			expectedFiles: map[string]string{
				"UniversalConfs/etc/resolv.conf": "create",
				"host3/etc/hosts":                "create",
				"host3/etc/crontab":              "create",
			},
			expectedFilesByHost: map[string][]string{
				"host1": {"UniversalConfs/etc/resolv.conf"},
				"host2": {"UniversalConfs/etc/resolv.conf"},
				"host3": {"host3/etc/hosts", "host3/etc/crontab"},
				"host4": {"UniversalConfs/etc/resolv.conf"},
			},
		},
		{
			name:          "No Commit Files",
			commitFiles:   map[string]string{},
			expectedHosts: []string{},
			expectedFiles: map[string]string{},
			expectedFilesByHost: map[string][]string{
				"": {""},
			},
		},
		{
			name: "Commit Files in Root of Repo",
			commitFiles: map[string]string{
				".example-file":   "create",
				"host3/etc/fstab": "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{},
			hostOverride:         "",
			expectedHosts:        []string{"host3"},
			expectedFiles: map[string]string{
				"host3/etc/fstab": "create",
			},
			expectedFilesByHost: map[string][]string{
				"host3": {"host3/etc/fstab"},
			},
		},
		{
			name: "Same File Between Universal And Host",
			commitFiles: map[string]string{
				"UniversalConfs/etc/issue": "create",
				"host2/etc/issue":          "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{
				"host2": {
					"UniversalConfs/etc/issue": {},
				},
			},
			expectedHosts: []string{"host1", "host2", "host4"},
			expectedFiles: map[string]string{
				"UniversalConfs/etc/issue": "create",
				"host2/etc/issue":          "create",
			},
			expectedFilesByHost: map[string][]string{
				"host1": {"UniversalConfs/etc/issue"},
				"host2": {"host2/etc/issue"},
				"host4": {"UniversalConfs/etc/issue"},
			},
		},
	}

	// Loop over each test case
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// Call the function under test
			allDeploymentHosts, allDeploymentFiles, filesByHost := filterHostsAndFiles(test.deniedUniversalFiles, test.commitFiles, test.hostOverride)

			// Validate the hosts
			if len(allDeploymentHosts) != len(test.expectedHosts) {
				t.Errorf("Expected %v hosts, got %v", test.expectedHosts, allDeploymentHosts)
			}
			if !compareArrays(test.expectedHosts, allDeploymentHosts) {
				t.Errorf("Expected deployment hosts %v, but got %v", test.expectedHosts, allDeploymentHosts)
			}

			// Validate the files
			for file, action := range test.expectedFiles {
				_, expectedFileExistsInOutput := allDeploymentFiles[file]
				if !expectedFileExistsInOutput {
					t.Errorf("Expected file '%s', but got nothing", file)
				}
				outputFileAction := allDeploymentFiles[file]
				if outputFileAction != action {
					t.Errorf("Expected action '%s' for file '%s', but got action '%s'", action, file, outputFileAction)
				}
			}

			// Validate files per host
			for _, endpointName := range allDeploymentHosts {
				expectedDeploymentFiles := test.expectedFilesByHost[endpointName]
				deploymentFiles := filesByHost[endpointName]

				if !compareArrays(expectedDeploymentFiles, deploymentFiles) {
					t.Errorf("Host %s: expected files %v, but got %v", endpointName, expectedDeploymentFiles, deploymentFiles)
				}
			}
		})
	}
}

func TestParseFileContent(t *testing.T) {
	type TestCase struct {
		name                string
		allDeploymentFiles  map[string]string
		rawFileContent      map[string][]byte
		expectedallFileMeta map[string]FileInfo
		expectedallFileData map[string][]byte
		expectedErr         bool
	}
	testCases := []TestCase{
		{
			name: "Standard single input",
			allDeploymentFiles: map[string]string{
				"host1/etc/file1.conf": "create",
			},
			rawFileContent: map[string][]byte{
				"host1/etc/file1.conf": []byte(`#|^^^|#
{
  "FileOwnerGroup": "root:root",
  "FilePermissions": 644,
  "Dependencies": [
    "/etc/file2.conf"
  ],
  "Install": [
    "apt-get install pkg1 -y"
  ],
  "Checks": [
    "ip a | grep ens18"
  ],
  "Reload": [
    "systemctl restart service1",
	"systemctl is-active service1"
  ]
}
#|^^^|#
some data here
more data here`),
			},
			expectedallFileMeta: map[string]FileInfo{
				"host1/etc/file1.conf": {
					hash:            "72fd888f1aaeea80dd9d8da0082e2c2f6df9c796175b27066c2f71872547b8a9",
					targetFilePath:  "/etc/file1.conf",
					action:          "create",
					ownerGroup:      "root:root",
					permissions:     644,
					fileSize:        29,
					linkTarget:      "",
					dependencies:    []string{"/etc/file2.conf"},
					installOptional: true,
					install:         []string{"apt-get install pkg1 -y"},
					checksRequired:  true,
					checks:          []string{"ip a | grep ens18"},
					reloadRequired:  true,
					reload:          []string{"systemctl restart service1", "systemctl is-active service1"},
				},
			},
			expectedallFileData: map[string][]byte{
				"72fd888f1aaeea80dd9d8da0082e2c2f6df9c796175b27066c2f71872547b8a9": []byte(`some data here
more data here`),
			},
			expectedErr: false,
		},
		{
			name: "Standard directory metadata input",
			allDeploymentFiles: map[string]string{
				"host1/var/www/site1/" + directoryMetadataFileName: "dirModify",
			},
			rawFileContent: map[string][]byte{
				"host1/var/www/site1/" + directoryMetadataFileName: []byte(`#|^^^|#
{
  "FileOwnerGroup": "root:www-data",
  "FilePermissions": 775,
  "Install": [
    "apt-get install nginx -y"
  ],
  "Checks": [
    "ss -taplnu | grep 443"
  ],
  "Reload": [
    "systemctl restart php8.3-fpm",
	"systemctl is-active php8.3-fpm"
  ]
}
#|^^^|#
`),
			},
			expectedallFileMeta: map[string]FileInfo{
				"host1/var/www/site1/" + directoryMetadataFileName: {
					hash:            "",
					targetFilePath:  "/var/www/site1",
					action:          "dirModify",
					ownerGroup:      "root:www-data",
					permissions:     775,
					fileSize:        0,
					linkTarget:      "",
					dependencies:    []string{},
					installOptional: true,
					install:         []string{"apt-get install nginx -y"},
					checksRequired:  true,
					checks:          []string{"ss -taplnu | grep 443"},
					reloadRequired:  true,
					reload:          []string{"systemctl restart php8.3-fpm", "systemctl is-active php8.3-fpm"},
				},
			},
			expectedallFileData: map[string][]byte{"": {}},
			expectedErr:         false,
		},
		{
			name: "Standard delete input",
			allDeploymentFiles: map[string]string{
				"host1/etc/exm.conf": "delete",
			},
			rawFileContent: map[string][]byte{
				"host1/etc/exm.conf": {},
			},
			expectedallFileMeta: map[string]FileInfo{
				"host1/etc/exm.conf": {
					hash:            "",
					targetFilePath:  "/etc/exm.conf",
					action:          "delete",
					ownerGroup:      "",
					permissions:     0,
					fileSize:        0,
					linkTarget:      "",
					dependencies:    []string{""},
					installOptional: false,
					install:         []string{""},
					checksRequired:  false,
					checks:          []string{""},
					reloadRequired:  false,
					reload:          []string{""},
				},
			},
			expectedallFileData: map[string][]byte{},
			expectedErr:         false,
		},
		{
			name:                "No input",
			allDeploymentFiles:  map[string]string{},
			rawFileContent:      map[string][]byte{},
			expectedallFileMeta: map[string]FileInfo{},
			expectedallFileData: map[string][]byte{},
			expectedErr:         true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			allFileMeta, allFileData, err := parseFileContent(test.allDeploymentFiles, test.rawFileContent)

			if err != nil && !test.expectedErr {
				t.Fatalf("Expected no error - but got error '%v'", err)
			}
			if err == nil && test.expectedErr {
				t.Fatalf("Expected err '%v' - but got no error", test.expectedErr)
			}

			if fmt.Sprintf("%v", test.expectedallFileMeta) != fmt.Sprintf("%v", allFileMeta) {
				t.Errorf("Expected metadata does not match output metadata:\nOutput:\n%v\n\nExpected Output:\n%v\n", allFileMeta, test.expectedallFileMeta)
			}
			if fmt.Sprintf("%v", test.expectedallFileData) != fmt.Sprintf("%v", allFileData) {
				t.Errorf("Expected data does not match output data:\nOutput:\n%v\n\nExpected Output:\n%v\n", allFileData, test.expectedallFileData)
			}
		})
	}
}

func compareArrays(array1, array2 []string) (arraysIdentical bool) {
	arraysIdentical = false

	// Quick check on length
	if len(array1) != len(array2) {
		return
	}

	// Sort both arrays
	sort.Strings(array1)
	sort.Strings(array2)

	// Compare sorted arrays element by element
	for i := range array1 {
		if array1[i] != array2[i] {
			return
		}
	}

	// They are the same
	arraysIdentical = true
	return
}

func TestHandleFileDependencies(t *testing.T) {
	testCases := []struct {
		name                string
		hostDeploymentFiles []string
		allFileMeta         map[string]FileInfo
		expected            []string
		expectErr           bool
		expectedNoOutput    bool
	}{
		{
			name:                "Correct lexicography order",
			hostDeploymentFiles: []string{"aaaa", "452dddd", "043cccc", "001bbbb", "010ffff", "002eeee"},
			allFileMeta: map[string]FileInfo{
				"010ffff": {
					dependencies: []string{"043cccc", "452dddd"},
				},
				"aaaa": {
					dependencies: []string{"010ffff"},
				},
				"452dddd": {
					dependencies: []string{},
				},
				"001bbbb": {
					dependencies: []string{},
				},
				"002eeee": {
					dependencies: []string{},
				},
				"043cccc": {
					dependencies: []string{},
				},
			},
			expected:  []string{"001bbbb", "002eeee", "043cccc", "452dddd", "010ffff", "aaaa"},
			expectErr: false,
		},
		{
			name:                "Correct lexicography order different input order",
			hostDeploymentFiles: []string{"043cccc", "aaaa", "010ffff", "001bbbb", "002eeee", "452dddd"},
			allFileMeta: map[string]FileInfo{
				"aaaa": {
					dependencies: []string{"010ffff"},
				},
				"452dddd": {
					dependencies: []string{},
				},
				"043cccc": {
					dependencies: []string{},
				},
				"001bbbb": {
					dependencies: []string{},
				},
				"002eeee": {
					dependencies: []string{},
				},
				"010ffff": {
					dependencies: []string{"043cccc", "452dddd"},
				},
			},
			expected:  []string{"001bbbb", "002eeee", "043cccc", "452dddd", "010ffff", "aaaa"},
			expectErr: false,
		},
		{
			name:                "Valid dependency order",
			hostDeploymentFiles: []string{"file1", "file2", "file3", "file4", "file5"},
			allFileMeta: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2", "file3"},
				},
				"file2": {
					dependencies: []string{"file3"},
				},
				"file5": {
					dependencies: []string{},
				},
				"file4": {
					dependencies: []string{"file1"},
				},
				"file3": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file3", "file5", "file2", "file1", "file4"},
			expectErr: false,
		},
		{
			name:                "Valid dependency order different input order",
			hostDeploymentFiles: []string{"file2", "file5", "file4", "file3", "file1"},
			allFileMeta: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2", "file3"},
				},
				"file2": {
					dependencies: []string{"file3"},
				},
				"file5": {
					dependencies: []string{},
				},
				"file4": {
					dependencies: []string{"file1"},
				},
				"file3": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file3", "file5", "file2", "file1", "file4"},
			expectErr: false,
		},
		{
			name:                "Valid dependency order Real Paths",
			hostDeploymentFiles: []string{"host1/etc/hosts", "host1/etc/apt/sources.list", "host1/etc/rsyslog.conf", "host1/etc/nginx/nginx.conf", "host1/etc/resolv.conf", "host1/etc/network/interfaces", "host1/etc/apt/apt.conf.d/00aptproxy"},
			allFileMeta: map[string]FileInfo{
				"host1/etc/nginx/nginx.conf": {
					dependencies: []string{"host1/etc/apt/sources.list"},
				},
				"host1/etc/apt/sources.list": {
					dependencies: []string{"host1/etc/apt/apt.conf.d/00aptproxy", "host1/etc/network/interfaces", "host1/etc/resolv.conf"},
				},
				"host1/etc/network/interfaces": {
					dependencies: []string{},
				},
				"host1/etc/hosts": {
					dependencies: []string{},
				},
				"host1/etc/rsyslog.conf": {
					dependencies: []string{"host1/etc/apt/sources.list"},
				},
				"host1/etc/resolv.conf": {
					dependencies: []string{"host1/etc/network/interfaces"},
				},
				"host1/etc/apt/apt.conf.d/00aptproxy": {
					dependencies: []string{"host1/etc/network/interfaces"},
				},
			},
			expected:  []string{"host1/etc/hosts", "host1/etc/network/interfaces", "host1/etc/apt/apt.conf.d/00aptproxy", "host1/etc/resolv.conf", "host1/etc/apt/sources.list", "host1/etc/nginx/nginx.conf", "host1/etc/rsyslog.conf"},
			expectErr: false,
		},
		{
			name:                "Non-Present Dependencies",
			hostDeploymentFiles: []string{"/etc/rsyslog.conf", "/etc/nginx/nginx.conf", "/etc/apt/sources.list"},
			allFileMeta: map[string]FileInfo{
				"/etc/nginx/nginx.conf": {
					dependencies: []string{"/etc/apt/sources.list"},
				},
				"/etc/apt/sources.list": {
					dependencies: []string{"/etc/apt/apt.conf.d/00aptproxy", "/etc/network/interfaces", "/etc/resolv.conf"},
				},
				"/etc/rsyslog.conf": {
					dependencies: []string{"/etc/apt/sources.list"},
				},
			},
			expected:  []string{"/etc/apt/sources.list", "/etc/nginx/nginx.conf", "/etc/rsyslog.conf"},
			expectErr: false,
		},
		{
			name:                "Circular dependency",
			hostDeploymentFiles: []string{"file1", "file2", "file3", "file4"},
			allFileMeta: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2"},
				},
				"file2": {
					dependencies: []string{"file4", "file3"},
				},
				"file3": {
					dependencies: []string{"file1"},
				},
				"file4": {
					dependencies: []string{},
				},
			},
			expected:         nil,
			expectErr:        true,
			expectedNoOutput: true,
		},
		{
			name:                "Circular dependency larger loop",
			hostDeploymentFiles: []string{"file4", "file2", "file1", "file3", "file5", "file6"},
			allFileMeta: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2"},
				},
				"file2": {
					dependencies: []string{"file3"},
				},
				"file3": {
					dependencies: []string{"file4"},
				},
				"file4": {
					dependencies: []string{"file5"},
				},
				"file5": {
					dependencies: []string{"file6"},
				},
				"file6": {
					dependencies: []string{"file1"},
				},
			},
			expected:         nil,
			expectErr:        true,
			expectedNoOutput: true,
		},
		{
			name:                "No dependencies",
			hostDeploymentFiles: []string{"file3", "file2", "file1"},
			allFileMeta: map[string]FileInfo{
				"file1": {
					dependencies: []string{},
				},
				"file2": {
					dependencies: []string{},
				},
				"file3": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file1", "file2", "file3"},
			expectErr: false,
		},
		{
			name:                "Single file with dependencies",
			hostDeploymentFiles: []string{"file1", "file2"},
			allFileMeta: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2"},
				},
				"file2": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file2", "file1"},
			expectErr: false,
		},
		{
			name:                "No input",
			hostDeploymentFiles: []string{},
			allFileMeta:         map[string]FileInfo{},
			expected:            []string{},
			expectErr:           false,
			expectedNoOutput:    false,
		},
	}

	// Loop over all test cases
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result, err := handleFileDependencies(test.hostDeploymentFiles, test.allFileMeta)

			// Check: error, output array, and output validity
			if test.expectedNoOutput && result != nil {
				t.Fatalf("expected no output, got '%v'", result)
			} else if !test.expectedNoOutput && result == nil {
				t.Fatalf("expected output, got nil")
			} else if test.expectErr && err == nil {
				t.Fatalf("expected error, got nil")
			} else if !test.expectErr && err != nil {
				t.Fatalf("expected no error, got '%v'", err)
			} else if !compareArrays(result, test.expected) {
				t.Errorf("expected '%v', got '%v'", test.expected, result)
			}
		})
	}
}

func TestCreateReloadGroups(t *testing.T) {
	testCases := []struct {
		name        string
		fileList    []string
		allFileMeta map[string]FileInfo
		expected    DeploymentList
	}{
		{
			name:     "All Identical Commands",
			fileList: []string{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			allFileMeta: map[string]FileInfo{
				"host1/etc/nginx/nginx.conf": {
					reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					reloadRequired: true,
				},
				"host1/etc/nginx/conf.d/site1.conf": {
					reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					reloadRequired: true,
				},
				"host1/etc/nginx/conf.d/site2.conf": {
					reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					reloadRequired: true,
				},
			},
			expected: DeploymentList{
				files: []string{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
				reloadIDtoFile: map[string][]string{
					"W3N5c3RlbWN0bCByZXN0YXJ0IG5naW54IHN5c3RlbWN0bCBpcy1hY3RpdmUgbmdpbnhd": {"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
				},
				fileToReloadID: map[string]string{
					"host1/etc/nginx/nginx.conf":        "W3N5c3RlbWN0bCByZXN0YXJ0IG5naW54IHN5c3RlbWN0bCBpcy1hY3RpdmUgbmdpbnhd",
					"host1/etc/nginx/conf.d/site1.conf": "W3N5c3RlbWN0bCByZXN0YXJ0IG5naW54IHN5c3RlbWN0bCBpcy1hY3RpdmUgbmdpbnhd",
					"host1/etc/nginx/conf.d/site2.conf": "W3N5c3RlbWN0bCByZXN0YXJ0IG5naW54IHN5c3RlbWN0bCBpcy1hY3RpdmUgbmdpbnhd",
				},
				reloadIDfileCount: map[string]int{
					"W3N5c3RlbWN0bCByZXN0YXJ0IG5naW54IHN5c3RlbWN0bCBpcy1hY3RpdmUgbmdpbnhd": 3,
				},
				reloadIDcommands: map[string][]string{
					"W3N5c3RlbWN0bCByZXN0YXJ0IG5naW54IHN5c3RlbWN0bCBpcy1hY3RpdmUgbmdpbnhd": {"systemctl restart nginx", "systemctl is-active nginx"},
				},
			},
		},
		{
			name:     "All Single Custom Group Names Different Reloads",
			fileList: []string{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			allFileMeta: map[string]FileInfo{
				"host1/etc/nginx/nginx.conf": {
					reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					reloadRequired: true,
					reloadGroup:    "NGINX Service",
				},
				"host1/etc/nginx/conf.d/site1.conf": {
					reload:         []string{"nginx -t", "systemctl restart nginx"},
					reloadRequired: true,
					reloadGroup:    "NGINX Service",
				},
				"host1/etc/nginx/conf.d/site2.conf": {
					reload:         []string{"grep active /etc/nginx/conf.d/site2.conf"},
					reloadRequired: true,
					reloadGroup:    "NGINX Service",
				},
			},
			expected: DeploymentList{
				files: []string{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
				reloadIDtoFile: map[string][]string{
					"NGINX Service": {"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
				},
				fileToReloadID: map[string]string{
					"host1/etc/nginx/nginx.conf":        "NGINX Service",
					"host1/etc/nginx/conf.d/site1.conf": "NGINX Service",
					"host1/etc/nginx/conf.d/site2.conf": "NGINX Service",
				},
				reloadIDfileCount: map[string]int{
					"NGINX Service": 3,
				},
				reloadIDcommands: map[string][]string{
					"NGINX Service": {"systemctl restart nginx", "systemctl is-active nginx", "nginx -t", "grep active /etc/nginx/conf.d/site2.conf"},
				},
			},
		},
		{
			name:     "Commands and Custom One Group",
			fileList: []string{"file2", "file3", "file4", "file5"},
			allFileMeta: map[string]FileInfo{
				"file2": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file3": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file4": {
					reloadGroup: "Service1",
				},
				"file5": {
					reloadGroup: "Service1",
				},
			},
			expected: DeploymentList{
				files: []string{"file2", "file3", "file4", "file5"},
				reloadIDtoFile: map[string][]string{
					"Service1": {"file2", "file3", "file4", "file5"},
				},
				fileToReloadID: map[string]string{
					"file2": "Service1",
					"file3": "Service1",
					"file4": "Service1",
					"file5": "Service1",
				},
				reloadIDfileCount: map[string]int{
					"Service1": 4,
				},
				reloadIDcommands: map[string][]string{
					"Service1": {"systemctl restart service1", "systemctl is-active service1"},
				},
			},
		},
		{
			name:     "One File Out Of Group But Identical Reloads",
			fileList: []string{"file2", "file3", "file4", "file5"},
			allFileMeta: map[string]FileInfo{
				"file2": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
				},
				"file3": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file4": {
					reloadGroup: "Service1",
				},
				"file5": {
					reloadGroup: "Service1",
				},
			},
			expected: DeploymentList{
				files: []string{"file2", "file3", "file4", "file5"},
				reloadIDtoFile: map[string][]string{
					"Service1": {"file2", "file3", "file4", "file5"},
				},
				fileToReloadID: map[string]string{
					"file2": "Service1",
					"file3": "Service1",
					"file4": "Service1",
					"file5": "Service1",
				},
				reloadIDfileCount: map[string]int{
					"Service1": 4,
				},
				reloadIDcommands: map[string][]string{
					"Service1": {"systemctl restart service1", "systemctl is-active service1"},
				},
			},
		},
		{
			name:     "Single Custom Group Different Reloads and No Reloads",
			fileList: []string{"file2", "file3", "file4", "file5", "file6", "file7"},
			allFileMeta: map[string]FileInfo{
				"file2": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file3": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file4": {
					reloadGroup: "Service1",
				},
				"file5": {
					reloadGroup: "Service1",
				},
				"file7": {
					reload:         []string{"service1 checkconf", "service1 reload file7"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file6": {
					reload:      []string{"service1 checkconf"},
					reloadGroup: "Service1",
				},
			},
			expected: DeploymentList{
				files: []string{"file2", "file3", "file4", "file5", "file6", "file7"},
				reloadIDtoFile: map[string][]string{
					"Service1": {"file2", "file3", "file4", "file5", "file6", "file7"},
				},
				fileToReloadID: map[string]string{
					"file2": "Service1",
					"file3": "Service1",
					"file4": "Service1",
					"file5": "Service1",
					"file6": "Service1",
					"file7": "Service1",
				},
				reloadIDfileCount: map[string]int{
					"Service1": 6,
				},
				reloadIDcommands: map[string][]string{
					"Service1": {"systemctl restart service1", "systemctl is-active service1", "service1 checkconf", "service1 reload file7"},
				},
			},
		},
		{
			name:     "Commands and Custom Two Different Groups",
			fileList: []string{"file2", "file3", "file4", "file5", "file6"},
			allFileMeta: map[string]FileInfo{
				"file3": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file2": {
					reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					reloadRequired: true,
					reloadGroup:    "Service1",
				},
				"file4": {
					reloadGroup: "Service2",
				},
				"file6": {
					reload:         []string{"service2 check-conf", "systemctl restart service2", "systemctl is-active service2"},
					reloadRequired: true,
					reloadGroup:    "Service2",
				},
				"file5": {
					reloadGroup: "Service2",
				},
			},
			expected: DeploymentList{
				files: []string{"file2", "file3", "file4", "file5", "file6"},
				reloadIDtoFile: map[string][]string{
					"Service1": {"file2", "file3"},
					"Service2": {"file4", "file5", "file6"},
				},
				fileToReloadID: map[string]string{
					"file4": "Service2",
					"file2": "Service1",
					"file3": "Service1",
					"file6": "Service2",
					"file5": "Service2",
				},
				reloadIDfileCount: map[string]int{
					"Service1": 2,
					"Service2": 3,
				},
				reloadIDcommands: map[string][]string{
					"Service1": {"systemctl restart service1", "systemctl is-active service1"},
					"Service2": {"service2 check-conf", "systemctl restart service2", "systemctl is-active service2"},
				},
			},
		},
		{
			name:     "Custom Group No Reloads",
			fileList: []string{"file3", "file2"},
			allFileMeta: map[string]FileInfo{
				"file3": {
					reloadGroup: "Service1",
				},
				"file2": {
					reloadGroup: "Service1",
				},
			},
			expected: DeploymentList{
				files: []string{"file3", "file2"},
				reloadIDtoFile: map[string][]string{
					"Service1": {"file3", "file2"},
				},
				fileToReloadID: map[string]string{
					"file3": "Service1",
					"file2": "Service1",
				},
				reloadIDfileCount: map[string]int{
					"Service1": 2,
				},
				reloadIDcommands: map[string][]string{
					"Service1": {},
				},
			},
		},
		{
			name:     "No Groups No Reloads",
			fileList: []string{"file3", "file2"},
			allFileMeta: map[string]FileInfo{
				"file2": {},
				"file3": {},
			},
			expected: DeploymentList{
				files:             []string{"file3", "file2"},
				reloadIDtoFile:    map[string][]string{},
				fileToReloadID:    map[string]string{},
				reloadIDfileCount: map[string]int{},
				reloadIDcommands:  map[string][]string{},
			},
		},
		{
			name:        "No Input",
			fileList:    []string{},
			allFileMeta: map[string]FileInfo{},
			expected: DeploymentList{
				files:             []string{},
				reloadIDtoFile:    map[string][]string{},
				fileToReloadID:    map[string]string{},
				reloadIDfileCount: map[string]int{},
				reloadIDcommands:  map[string][]string{},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			outputDeploymentList := createReloadGroups(test.fileList, test.allFileMeta)

			if !compareArrays(outputDeploymentList.files, test.expected.files) {
				t.Errorf("Files List: expected does not match output:\nExpected:\n%v\n\nOutput:\n%v\n", test.expected.files, outputDeploymentList.files)
			}

			if !compareSliceMaps(outputDeploymentList.reloadIDtoFile, test.expected.reloadIDtoFile) {
				t.Errorf("ReloadIDtoFile: expected does not match output:\nExpected:\n%v\n\nOutput:\n%v\n", test.expected.reloadIDtoFile, outputDeploymentList.reloadIDtoFile)
			}

			if !compareStringMaps(outputDeploymentList.fileToReloadID, test.expected.fileToReloadID) {
				t.Errorf("FileToReloadID: expected does not match output:\nExpected:\n%v\n\nOutput:\n%v\n", test.expected.fileToReloadID, outputDeploymentList.fileToReloadID)
			}

			if !compareIntMaps(outputDeploymentList.reloadIDfileCount, test.expected.reloadIDfileCount) {
				t.Errorf("ReloadIDfileCount: expected does not match output:\nExpected:\n%v\n\nOutput:\n%v\n", test.expected.reloadIDfileCount, outputDeploymentList.reloadIDfileCount)
			}

			if !compareSliceMaps(outputDeploymentList.reloadIDcommands, test.expected.reloadIDcommands) {
				t.Errorf("ReloadIDcommands: expected does not match output:\nExpected:\n%v\n\nOutput:\n%v\n", test.expected.reloadIDcommands, outputDeploymentList.reloadIDcommands)
			}
		})
	}
}

func compareSliceMaps(map1, map2 map[string][]string) bool {
	// First check if the lengths of the maps are equal
	if len(map1) != len(map2) {
		return false
	}

	// Check if all keys and their associated values are equal
	for key, val1 := range map1 {
		val2, ok := map2[key]
		if !ok {
			// Key doesn't exist in map2
			return false
		}

		if !compareArrays(val1, val2) {
			return false
		}
	}

	// If we passed all checks, the maps are equal
	return true
}

func compareStringMaps(a, b map[string]string) bool {
	// If lengths are different, maps are not equal
	if len(a) != len(b) {
		return false
	}

	// Check if all key-value pairs are equal
	for key, value := range a {
		if bValue, exists := b[key]; !exists || bValue != value {
			return false
		}
	}
	return true
}

func compareIntMaps(a, b map[string]int) bool {
	// If lengths are different, they are not equal
	if len(a) != len(b) {
		return false
	}

	// Check if all key-value pairs are equal (we assume the "map" comparison)
	for key, value := range a {
		if bValue, exists := b[key]; !exists || bValue != value {
			return false
		}
	}

	return true
}
