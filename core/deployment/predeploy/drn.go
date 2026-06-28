package predeploy

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/core/drn/resolve"
	"scmp/internal/config"
	"scmp/internal/crypto"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"sync"
	"sync/atomic"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Uses the HostFiles object per host to find and replace DRNs
func HandleDRNs(ctx context.Context,
	tree *object.Tree,
	allHostFiles map[str.RepoRootDir]*deployment.HostFiles,
	hostInfo map[str.RepoRootDir]config.EndpointInfo,
) (err error) {
	// Retrieve required deployment options
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSDRN)

	treeReader := gitinternal.NewTreeReader(tree)

	// Replacer for all hosts/files (concurrent safe)
	drnReplacer := resolve.NewReplacer(cfg.RepositoryPath, treeReader)

	// Extraction/Validation (concurrent)
	var extractWG sync.WaitGroup
	for hostAlias, hostFiles := range allHostFiles {
		extractWG.Add(1)
		go extractHostDRNs(&extractWG, drnReplacer, hostAlias, hostFiles)
	}
	extractWG.Wait()

	// Early return if no DRNs are in this deployment
	if drnReplacer.ExtractedCount() == 0 {
		// No-op
		return
	}

	// Lookup Phase (serial)
	err = drnReplacer.ResolveAll(ctx, hostInfo)
	if err != nil {
		err = fmt.Errorf("resolve: %w", err)
		return
	}

	// Replace Phase (concurrent)
	var errorCount atomic.Uint64
	var replaceWG sync.WaitGroup
	for hostAlias, hostFiles := range allHostFiles {
		replaceWG.Add(1)
		go replaceHostDRNs(ctx, &replaceWG, &errorCount, drnReplacer, hostAlias, hostFiles)
	}
	replaceWG.Wait()

	if errorCount.Load() > 0 {
		err = fmt.Errorf("encountered %d error(s): see log(s) for details", errorCount.Load())
		return
	}
	return
}

// Extracts and validates (concurrently) all DRNs for all host files
func extractHostDRNs(wg *sync.WaitGroup,
	drnReplacer *resolve.Replacer,
	hostAlias str.RepoRootDir, hostFiles *deployment.HostFiles,
) {
	defer wg.Done()

	var fileWG sync.WaitGroup
	concurrencyLimiter := make(chan struct{}, 4)
	for _, file := range hostFiles.GetUnorderedList() {

		fileWG.Go(func() {
			concurrencyLimiter <- struct{}{}
			defer func() { <-concurrencyLimiter }()
			info := hostFiles.GetFileInfo(file)
			data := hostFiles.GetFileData(info.Hash)
			drnReplacer.ExtractDRNs(hostAlias, file, data)
			drnReplacer.ExtractHeaderDRNs(hostAlias, file, info)
		})
	}
	fileWG.Wait()
}

// Replaces all DRNs for all host files
func replaceHostDRNs(ctx context.Context, wg *sync.WaitGroup,
	errorCount *atomic.Uint64,
	drnReplacer *resolve.Replacer,
	hostAlias str.RepoRootDir, hostFiles *deployment.HostFiles,
) {
	defer wg.Done()

	var fileWG sync.WaitGroup
	concurrencyLimiter := make(chan struct{}, 4)
	for _, file := range hostFiles.GetUnorderedList() {

		fileWG.Add(1)
		go replaceFileDRNs(ctx, &fileWG, concurrencyLimiter, errorCount, drnReplacer, hostAlias, file, hostFiles)
	}
	fileWG.Wait()
}

// Replaces all DRNs for files in a single host reload group
func replaceFileDRNs(ctx context.Context, wg *sync.WaitGroup, concurrencyLimiter chan struct{},
	errorCount *atomic.Uint64,
	drnReplacer *resolve.Replacer,
	hostAlias str.RepoRootDir, file str.LocalRepoPath, hostFiles *deployment.HostFiles,
) {
	defer wg.Done()

	concurrencyLimiter <- struct{}{}
	defer func() { <-concurrencyLimiter }()

	info := hostFiles.GetFileInfo(file)
	data := hostFiles.GetFileData(info.Hash)

	newData, dataReplaceMade, err := drnReplacer.ReplaceDRNs(hostAlias, file, data)
	if err != nil {
		logctx.LogStdErr(ctx, "DRN file data replacement: %w\n", err)
		errorCount.Add(1)
		return
	}
	newHeader, metaReplaceMade, err := drnReplacer.ReplaceHeaderDRNs(hostAlias, file, info)
	if err != nil {
		logctx.LogStdErr(ctx, "DRN file metadata replacement: %w\n", err)
		errorCount.Add(1)
		return
	}

	if dataReplaceMade {
		// Hash new contents
		newHeader.Hash = str.FileID(crypto.SHA256Sum(newData))
		hostFiles.StoreDataOnce(newHeader.Hash, newData)

		// Change hash pointer to new contents
		hostFiles.ChangeFileDataPointer(file, newHeader.Hash)
	}

	if metaReplaceMade || dataReplaceMade {
		// Update path metadata
		hostFiles.SetFileMetadata(file, newHeader)
	}
}
