package predeploy

import (
	"context"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"slices"
	"testing"
)

func TestMapDeniedUniversalFiles(t *testing.T) {
	// Mock Global
	config := config.Config{
		HostInfo: map[str.RepoRootDir]config.EndpointInfo{
			"host1": {
				UniversalGroups: map[str.RepoRootDir]struct{}{
					"UniversalConfs_Service1": {},
				},
			},
			"host2": {
				UniversalGroups: map[str.RepoRootDir]struct{}{
					"UniversalConfs_OtherServers": {},
				},
			},
			"host3": {
				UniversalGroups: map[str.RepoRootDir]struct{}{
					"": {},
				},
			},
		},
		UniversalDirectory: "UniversalConfs",
	}
	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())

	ctx = context.WithValue(ctx, global.ConfKey, config)

	// Test Data
	allHostsFiles := map[str.RepoRootDir]map[str.RemotePath]struct{}{
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
	universalFiles := map[str.RepoRootDir]map[str.RemotePath]struct{}{
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
	deniedUniversalFiles := MapDeniedUniversalFiles(ctx, allHostsFiles, universalFiles)

	// Expected result
	expectedDeniedFiles := map[str.RepoRootDir]map[str.LocalRepoPath]struct{}{
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

func TestFilterHostsAndFiles(t *testing.T) {
	// Mock ctx
	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())
	ctx = context.WithValue(ctx, global.OpsKey, config.Opts{IgnoreDeploymentState: false})

	hostInfo := map[str.RepoRootDir]config.EndpointInfo{
		"host1": {
			DeploymentState: "online",
			IgnoreUniversal: false,
			UniversalGroups: map[str.RepoRootDir]struct{}{"UniversalConfs_Service1": {}, "UniversalConfs": {}},
			EndpointName:    "host1",
		},
		"host2": {
			DeploymentState: "",
			IgnoreUniversal: false,
			UniversalGroups: map[str.RepoRootDir]struct{}{"UniversalConfs_Service2": {}, "UniversalConfs": {}},
			EndpointName:    "host2",
		},
		"host3": {
			DeploymentState: "go",
			IgnoreUniversal: true,
			UniversalGroups: map[str.RepoRootDir]struct{}{"": {}},
			EndpointName:    "host3",
		},
		"host4": {
			DeploymentState: "",
			IgnoreUniversal: false,
			UniversalGroups: map[str.RepoRootDir]struct{}{"UniversalConfs": {}},
			EndpointName:    "host4",
		},
		"host5": {
			DeploymentState: "offline",
			IgnoreUniversal: false,
			UniversalGroups: map[str.RepoRootDir]struct{}{"UniversalConfs": {}},
			EndpointName:    "host5",
		},
	}

	// Test cases
	type TestCase struct {
		name                 string
		commitFiles          map[str.LocalRepoPath]str.DeployAction
		deniedUniversalFiles map[str.RepoRootDir]map[str.LocalRepoPath]struct{}
		hostOverride         string
		expectedHosts        []str.RepoRootDir
		expectedFiles        map[str.LocalRepoPath]str.DeployAction
		expectedFilesByHost  map[str.RepoRootDir][]str.LocalRepoPath
	}
	testCases := []TestCase{
		{
			name: "Standard Deployment Only Host Files",
			commitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/resolv.conf":      deployment.ActionCreate,
				"host1/etc/hosts":            deployment.ActionCreate,
				"host2/etc/nginx/nginx.conf": deployment.ActionCreate,
			},
			expectedHosts: []str.RepoRootDir{"host1", "host2"},
			expectedFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/resolv.conf":      deployment.ActionCreate,
				"host1/etc/hosts":            deployment.ActionCreate,
				"host2/etc/nginx/nginx.conf": deployment.ActionCreate,
			},
			expectedFilesByHost: map[str.RepoRootDir][]str.LocalRepoPath{
				"host1": {"host1/etc/resolv.conf", "host1/etc/hosts"},
				"host2": {"host2/etc/nginx/nginx.conf"},
			},
		},
		{
			name: "Host Override Single Host",
			commitFiles: map[str.LocalRepoPath]str.DeployAction{
				"host1/etc/hosts":              deployment.ActionCreate,
				"host2/etc/network/interfaces": deployment.ActionCreate,
				"host3/etc/rsyslog.conf":       deployment.ActionCreate,
			},
			deniedUniversalFiles: map[str.RepoRootDir]map[str.LocalRepoPath]struct{}{
				"host1": {
					"UniversalConfs/etc/some-file": {},
				},
			},
			hostOverride:  "host3",
			expectedHosts: []str.RepoRootDir{"host3"},
			expectedFiles: map[str.LocalRepoPath]str.DeployAction{
				"host3/etc/rsyslog.conf": deployment.ActionCreate,
			},
			expectedFilesByHost: map[str.RepoRootDir][]str.LocalRepoPath{
				"host3": {"host3/etc/rsyslog.conf"},
			},
		},
		{
			name: "Host Ignores Universal",
			commitFiles: map[str.LocalRepoPath]str.DeployAction{
				"UniversalConfs/etc/resolv.conf": deployment.ActionCreate,
				"host3/etc/hosts":                deployment.ActionCreate,
				"host3/etc/crontab":              deployment.ActionCreate,
			},
			deniedUniversalFiles: map[str.RepoRootDir]map[str.LocalRepoPath]struct{}{
				"host3": {
					"UniversalConfs/etc/hosts": {},
				},
			},
			hostOverride:  "",
			expectedHosts: []str.RepoRootDir{"host1", "host2", "host3", "host4"},
			expectedFiles: map[str.LocalRepoPath]str.DeployAction{
				"UniversalConfs/etc/resolv.conf": deployment.ActionCreate,
				"host3/etc/hosts":                deployment.ActionCreate,
				"host3/etc/crontab":              deployment.ActionCreate,
			},
			expectedFilesByHost: map[str.RepoRootDir][]str.LocalRepoPath{
				"host1": {"UniversalConfs/etc/resolv.conf"},
				"host2": {"UniversalConfs/etc/resolv.conf"},
				"host3": {"host3/etc/hosts", "host3/etc/crontab"},
				"host4": {"UniversalConfs/etc/resolv.conf"},
			},
		},
		{
			name:          "No Commit Files",
			commitFiles:   map[str.LocalRepoPath]str.DeployAction{},
			expectedHosts: []str.RepoRootDir{},
			expectedFiles: map[str.LocalRepoPath]str.DeployAction{},
			expectedFilesByHost: map[str.RepoRootDir][]str.LocalRepoPath{
				"": {""},
			},
		},
		{
			name: "Commit Files in Root of Repo",
			commitFiles: map[str.LocalRepoPath]str.DeployAction{
				".example-file":   deployment.ActionCreate,
				"host3/etc/fstab": deployment.ActionCreate,
			},
			deniedUniversalFiles: map[str.RepoRootDir]map[str.LocalRepoPath]struct{}{},
			hostOverride:         "",
			expectedHosts:        []str.RepoRootDir{"host3"},
			expectedFiles: map[str.LocalRepoPath]str.DeployAction{
				"host3/etc/fstab": deployment.ActionCreate,
			},
			expectedFilesByHost: map[str.RepoRootDir][]str.LocalRepoPath{
				"host3": {"host3/etc/fstab"},
			},
		},
		{
			name: "Same File Between Universal And Host",
			commitFiles: map[str.LocalRepoPath]str.DeployAction{
				"UniversalConfs/etc/issue": deployment.ActionCreate,
				"host2/etc/issue":          deployment.ActionCreate,
			},
			deniedUniversalFiles: map[str.RepoRootDir]map[str.LocalRepoPath]struct{}{
				"host2": {
					"UniversalConfs/etc/issue": {},
				},
			},
			expectedHosts: []str.RepoRootDir{"host1", "host2", "host4"},
			expectedFiles: map[str.LocalRepoPath]str.DeployAction{
				"UniversalConfs/etc/issue": deployment.ActionCreate,
				"host2/etc/issue":          deployment.ActionCreate,
			},
			expectedFilesByHost: map[str.RepoRootDir][]str.LocalRepoPath{
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
			allDeploymentHosts, allDeploymentFiles, filesByHost := FilterHostsAndFiles(ctx, hostInfo, test.deniedUniversalFiles, test.commitFiles, test.hostOverride)

			// Validate the hosts
			if len(allDeploymentHosts) != len(test.expectedHosts) {
				t.Errorf("Expected %v hosts, got %v", test.expectedHosts, allDeploymentHosts)
			}
			if !str.CompareArrays(test.expectedHosts, allDeploymentHosts) {
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

				if !str.CompareArrays(expectedDeploymentFiles, deploymentFiles) {
					t.Errorf("Host %s: expected files %v, but got %v", endpointName, expectedDeploymentFiles, deploymentFiles)
				}
			}
		})
	}
}

func TestCreateReloadGroups(t *testing.T) {
	testCases := []struct {
		name             string
		fileList         []str.LocalRepoPath
		allFileMeta      map[str.LocalRepoPath]deployment.FileInfo
		expectFiles      []str.LocalRepoPath
		reloadIDtoFile   map[str.ReloadID][]str.LocalRepoPath
		fileToReloadID   map[str.LocalRepoPath]str.ReloadID
		reloadIDcommands map[str.ReloadID][]string
	}{
		{
			name:     "All Identical Commands",
			fileList: []str.LocalRepoPath{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/nginx/nginx.conf": {
					Reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					ReloadRequired: true,
				},
				"host1/etc/nginx/conf.d/site1.conf": {
					Reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					ReloadRequired: true,
				},
				"host1/etc/nginx/conf.d/site2.conf": {
					Reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					ReloadRequired: true,
				},
			},
			expectFiles: []str.LocalRepoPath{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			reloadIDtoFile: map[str.ReloadID][]str.LocalRepoPath{
				"c3lzdGVtY3RsIHJlc3RhcnQgbmdpbnh8c3lzdGVtY3RsIGlzLWFjdGl2ZSBuZ2lueA==": {"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			},
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"host1/etc/nginx/nginx.conf":        "c3lzdGVtY3RsIHJlc3RhcnQgbmdpbnh8c3lzdGVtY3RsIGlzLWFjdGl2ZSBuZ2lueA==",
				"host1/etc/nginx/conf.d/site1.conf": "c3lzdGVtY3RsIHJlc3RhcnQgbmdpbnh8c3lzdGVtY3RsIGlzLWFjdGl2ZSBuZ2lueA==",
				"host1/etc/nginx/conf.d/site2.conf": "c3lzdGVtY3RsIHJlc3RhcnQgbmdpbnh8c3lzdGVtY3RsIGlzLWFjdGl2ZSBuZ2lueA==",
			},
			reloadIDcommands: map[str.ReloadID][]string{
				"c3lzdGVtY3RsIHJlc3RhcnQgbmdpbnh8c3lzdGVtY3RsIGlzLWFjdGl2ZSBuZ2lueA==": {"systemctl restart nginx", "systemctl is-active nginx"},
			},
		},
		{
			name:     "All Single Custom Group Names Different Reloads",
			fileList: []str.LocalRepoPath{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/nginx/nginx.conf": {
					Reload:         []string{"systemctl restart nginx", "systemctl is-active nginx"},
					ReloadRequired: true,
					ReloadGroup:    "NGINX Service",
				},
				"host1/etc/nginx/conf.d/site1.conf": {
					Reload:         []string{"nginx -t", "systemctl restart nginx"},
					ReloadRequired: true,
					ReloadGroup:    "NGINX Service",
				},
				"host1/etc/nginx/conf.d/site2.conf": {
					Reload:         []string{"grep active /etc/nginx/conf.d/site2.conf"},
					ReloadRequired: true,
					ReloadGroup:    "NGINX Service",
				},
			},
			expectFiles: []str.LocalRepoPath{"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			reloadIDtoFile: map[str.ReloadID][]str.LocalRepoPath{
				"NGINX Service": {"host1/etc/nginx/nginx.conf", "host1/etc/nginx/conf.d/site1.conf", "host1/etc/nginx/conf.d/site2.conf"},
			},
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"host1/etc/nginx/nginx.conf":        "NGINX Service",
				"host1/etc/nginx/conf.d/site1.conf": "NGINX Service",
				"host1/etc/nginx/conf.d/site2.conf": "NGINX Service",
			},
			reloadIDcommands: map[str.ReloadID][]string{
				"NGINX Service": {"systemctl restart nginx", "systemctl is-active nginx", "nginx -t", "grep active /etc/nginx/conf.d/site2.conf"},
			},
		},
		{
			name:     "Commands and Custom One Group",
			fileList: []str.LocalRepoPath{"file2", "file3", "file4", "file5"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file2": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file3": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file4": {
					ReloadGroup: "Service1",
				},
				"file5": {
					ReloadGroup: "Service1",
				},
			},
			expectFiles: []str.LocalRepoPath{"file2", "file3", "file4", "file5"},
			reloadIDtoFile: map[str.ReloadID][]str.LocalRepoPath{
				"Service1": {"file2", "file3", "file4", "file5"},
			},
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file2": "Service1",
				"file3": "Service1",
				"file4": "Service1",
				"file5": "Service1",
			},
			reloadIDcommands: map[str.ReloadID][]string{
				"Service1": {"systemctl restart service1", "systemctl is-active service1"},
			},
		},
		{
			name:     "One File Out Of Group But Identical Reloads",
			fileList: []str.LocalRepoPath{"file2", "file3", "file4", "file5"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file2": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
				},
				"file3": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file4": {
					ReloadGroup: "Service1",
				},
				"file5": {
					ReloadGroup: "Service1",
				},
			},
			expectFiles: []str.LocalRepoPath{"file2", "file3", "file4", "file5"},
			reloadIDtoFile: map[str.ReloadID][]str.LocalRepoPath{
				"Service1": {"file2", "file3", "file4", "file5"},
			},
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file2": "Service1",
				"file3": "Service1",
				"file4": "Service1",
				"file5": "Service1",
			},
			reloadIDcommands: map[str.ReloadID][]string{
				"Service1": {"systemctl restart service1", "systemctl is-active service1"},
			},
		},
		{
			name:     "Single Custom Group Different Reloads and No Reloads",
			fileList: []str.LocalRepoPath{"file2", "file3", "file4", "file5", "file6", "file7"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file2": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file3": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file4": {
					ReloadGroup: "Service1",
				},
				"file5": {
					ReloadGroup: "Service1",
				},
				"file7": {
					Reload:         []string{"service1 checkconf", "service1 reload file7"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file6": {
					Reload:      []string{"service1 checkconf"},
					ReloadGroup: "Service1",
				},
			},
			expectFiles: []str.LocalRepoPath{"file2", "file3", "file4", "file5", "file6", "file7"},
			reloadIDtoFile: map[str.ReloadID][]str.LocalRepoPath{
				"Service1": {"file2", "file3", "file4", "file5", "file6", "file7"},
			},
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file2": "Service1",
				"file3": "Service1",
				"file4": "Service1",
				"file5": "Service1",
				"file6": "Service1",
				"file7": "Service1",
			},
			reloadIDcommands: map[str.ReloadID][]string{
				"Service1": {"systemctl restart service1", "systemctl is-active service1", "service1 checkconf", "service1 reload file7"},
			},
		},
		{
			name:     "Commands and Custom Two Different Groups",
			fileList: []str.LocalRepoPath{"file2", "file3", "file4", "file5", "file6"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file3": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file2": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file4": {
					ReloadGroup: "Service2",
				},
				"file6": {
					Reload:         []string{"service2 check-conf", "systemctl restart service2", "systemctl is-active service2"},
					ReloadRequired: true,
					ReloadGroup:    "Service2",
				},
				"file5": {
					ReloadGroup: "Service2",
				},
			},
			expectFiles: []str.LocalRepoPath{"file2", "file3", "file4", "file5", "file6"},
			reloadIDtoFile: map[str.ReloadID][]str.LocalRepoPath{
				"Service1": {"file2", "file3"},
				"Service2": {"file4", "file5", "file6"},
			},
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file4": "Service2",
				"file2": "Service1",
				"file3": "Service1",
				"file6": "Service2",
				"file5": "Service2",
			},
			reloadIDcommands: map[str.ReloadID][]string{
				"Service1": {"systemctl restart service1", "systemctl is-active service1"},
				"Service2": {"service2 check-conf", "systemctl restart service2", "systemctl is-active service2"},
			},
		},
		{
			name:     "Custom Group No Reloads",
			fileList: []str.LocalRepoPath{"file3", "file2"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file3": {
					ReloadGroup: "Service1",
				},
				"file2": {
					ReloadGroup: "Service1",
				},
			},
			expectFiles: []str.LocalRepoPath{"file3", "file2"},
			reloadIDtoFile: map[str.ReloadID][]str.LocalRepoPath{
				"Service1": {"file3", "file2"},
			},
			fileToReloadID: map[str.LocalRepoPath]str.ReloadID{
				"file3": "Service1",
				"file2": "Service1",
			},
			reloadIDcommands: map[str.ReloadID][]string{
				"Service1": {},
			},
		},
		{
			name:     "No Groups No Reloads",
			fileList: []str.LocalRepoPath{"file3", "file2"},
			allFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file2": {},
				"file3": {},
			},
			expectFiles:      []str.LocalRepoPath{"file3", "file2"},
			reloadIDtoFile:   map[str.ReloadID][]str.LocalRepoPath{},
			fileToReloadID:   map[str.LocalRepoPath]str.ReloadID{},
			reloadIDcommands: map[str.ReloadID][]string{},
		},
		{
			name:             "No Input",
			fileList:         []str.LocalRepoPath{},
			allFileMeta:      map[str.LocalRepoPath]deployment.FileInfo{},
			expectFiles:      []str.LocalRepoPath{},
			reloadIDtoFile:   map[str.ReloadID][]str.LocalRepoPath{},
			fileToReloadID:   map[str.LocalRepoPath]str.ReloadID{},
			reloadIDcommands: map[str.ReloadID][]string{},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// Prepare deploy files obj
			deployFiles, err := deployment.NewHostFiles()
			if err != nil {
				t.Fatalf("failed init host files obj: %v", err)
			}
			for path, meta := range test.allFileMeta {
				deployFiles.SetFileMetadata(path, meta)
			}

			expectDeploymentList := deployment.NewFileGroup(test.expectFiles)
			for reloadID, files := range test.reloadIDtoFile {
				expectDeploymentList.AppendFileToReloadID(reloadID, files...)

				seen := make(map[string]bool)
				for _, file := range files {
					info := deployFiles.GetFileInfo(file)

					for _, cmd := range info.Reload {
						if seen[cmd] {
							continue
						}
						expectDeploymentList.AppendCmdToReloadID(reloadID, file, cmd)
						seen[cmd] = true
					}
				}
			}
			expectDeploymentList.InitFiletoReloadID()
			expectDeploymentList.RecordReloadIDFileCount()

			outputDeploymentList := CreateReloadGroups(test.fileList, deployFiles)

			if !str.CompareArrays(outputDeploymentList.GetOrderedList(), expectDeploymentList.GetOrderedList()) {
				t.Fatalf("Files List: mismatch:\nExpected: %v\nGot:      %v",
					expectDeploymentList.GetOrderedList(), outputDeploymentList.GetOrderedList())
			}

			gotReloadIDList := outputDeploymentList.GetReloadIDs()
			expectReloadIDList := expectDeploymentList.GetReloadIDs()
			if !slices.Equal(gotReloadIDList, expectReloadIDList) {
				t.Fatalf("Reload IDs: mismatch:\nExpected: %v\nGot:      %v", expectReloadIDList, gotReloadIDList)
			}

			// Reload IDs are identical, use expected
			for _, reloadID := range expectReloadIDList {
				gotReloadFiles := outputDeploymentList.GetReloadIDFiles(reloadID)
				expectedReloadFiles := expectDeploymentList.GetReloadIDFiles(reloadID)

				if !slices.Equal(gotReloadFiles, expectedReloadFiles) {
					t.Errorf("Reload ID '%s' Files: mismatch:\nExpected: %v\nGot:      %v", reloadID, expectedReloadFiles, gotReloadFiles)
				}

				gotReloadCmds := outputDeploymentList.GetReloadIDCommands(reloadID)
				expectedReloadCmds := expectDeploymentList.GetReloadIDCommands(reloadID)
				if !slices.Equal(gotReloadCmds, expectedReloadCmds) {
					t.Errorf("Reload ID '%s' Commands: mismatch:\nExpected: %v\nGot:      %v", reloadID, expectedReloadCmds, gotReloadCmds)
				}

				gotReloadFileCnt := outputDeploymentList.GetReloadIDFileCount(reloadID)
				expectedReloadFileCnt := expectDeploymentList.GetReloadIDFileCount(reloadID)
				if gotReloadFileCnt != expectedReloadFileCnt {
					t.Errorf("Reload ID '%s' File Count: mismatch:\nExpected: %d\nGot:      %d", reloadID, expectedReloadFileCnt, gotReloadFileCnt)
				}
			}
		})
	}
}
