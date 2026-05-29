package cli

import "fmt"

func WOCLICmds(cfg *CommandSet) (err error) {
	cliOptsMutex.Lock()
	defer cliOptsMutex.Unlock()

	if cliOptsSet {
		err = fmt.Errorf("global already set")
		return
	}

	cliOptsOnce.Do(func() {
		cliOpts = cfg
		cliOptsSet = true
	})

	return
}
func GetCLICmds() *CommandSet {
	cliOptsMutex.RLock()
	defer cliOptsMutex.RUnlock()

	return cliOpts
}
