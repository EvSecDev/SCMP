package deployment

import (
	"fmt"
)

func NewHostFiles(allDeployFiles *AllFiles) (files *HostFiles, err error) {
	if allDeployFiles == nil {
		err = fmt.Errorf("uninitialized all files list")
		return
	}
	files = &HostFiles{
		GlobalFiles: allDeployFiles,
	}
	return
}
