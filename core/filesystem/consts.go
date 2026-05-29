package filesystem

import "scmp/internal/str"

const (
	MetaDelimiter          string            = "#|^^^|#"                              // Start and stop delimiter for repository file metadata header
	ArtifactPointerFileExt str.LocalRepoPath = ".remote-artifact"                     // file extension to identify 'pointer' files for artifact files
	DirMetaFileName        str.LocalRepoPath = ".directory_metadata_information.json" // hidden file to identify parent directories metadata
)
