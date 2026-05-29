package setup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/internal/logctx"
	"strings"
)

// If apparmor LSM is available on this system and running as root, auto install the profile - failures are not printed under normal verbosity
func AAProfile(ctx context.Context, repositoryPath string) {
	if repositoryPath == "" {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to install apparmor profile: missing repository-path\n")
		return
	}

	const appArmorProfilePath string = "/etc/apparmor.d/scmp-controller"
	appArmorProfile, err := installationConfigs.ReadFile("static-files/configurations/apparmor-profile")
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to retrieve configuration file from embedded filesystem: %v\n", err)
		return
	}

	executablePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to retrieve absolute executable path for profile installation: %v\n", err)
		return
	}

	// Inject variables into config
	newaaProf := strings.Replace(string(appArmorProfile), "=$executablePath", "="+executablePath, 1)
	newaaProf = strings.Replace(newaaProf, "=$repositoryPath", "="+repositoryPath, 1)
	newaaProf = strings.Replace(newaaProf, "=$aaProfPath", "="+appArmorProfilePath, 1)
	appArmorProfile = []byte(newaaProf)

	// Can't install apparmor profile without root/sudo
	if os.Geteuid() > 0 {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.WarnLog, "Need root permissions to install apparmor profile\n")
		return
	}

	// Check if apparmor /sys path exists
	systemAAPath := "/sys/kernel/security/apparmor/profiles"
	_, err = os.Stat(systemAAPath)
	if os.IsNotExist(err) {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "AppArmor not supported by this system\n")
		return
	} else if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to check if AppArmor is supported by this system: %v\n", err)
		return
	}

	// Write Apparmor Profile to /etc
	err = os.WriteFile(appArmorProfilePath, appArmorProfile, 0644)
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to write apparmor profile: %v\n", err)
		return
	}

	// Enact Profile
	command := exec.Command("apparmor_parser", "-r", appArmorProfilePath)
	_, err = command.CombinedOutput()
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to reload apparmor profile: %v\n", err)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Successfully installed AppArmor Profile\n")
}
