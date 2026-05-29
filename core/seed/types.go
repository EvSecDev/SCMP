package seed

// Keeping track of directories in menu
type DirectoryState struct {
	current string
	stack   []string
}

type RepoUserChoiceCache struct {
	ReloadCmd      map[string][]string
	ReloadCnt      map[string]int
	ArtifactExtDir map[string]int
}
