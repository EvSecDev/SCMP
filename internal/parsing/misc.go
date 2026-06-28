package parsing

import (
	"context"
	"regexp"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"
)

// Checks for user-chosen host/file override with given host/file
// Returns immediately if override is empty
func CheckForOverride(ctx context.Context, override string, current string, hostList map[str.RepoRootDir]config.EndpointInfo) (skip bool) {
	// Retrieve required deployment options
	opts := global.AssertFromContext[config.Opts](ctx, "options", global.OpsKey, "config.Opts")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSValidation)

	hostInfo, inputCheckIsAHost := hostList[str.RepoRootDir(current)]

	// If input is a host and state is offline and user did not request deployment state override, then skip
	if inputCheckIsAHost && hostInfo.DeploymentState == "offline" && !opts.IgnoreDeploymentState {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  host %s is currently offline\n", current)
		skip = true
		return
	}

	// Return early if no override
	if override == "" {
		return
	}

	// Allow current item if item is part of a group
	// Only applies to host overrides, but shouldn't affect file overrides
	group, currentItemIsPartofGroup := hostInfo.UniversalGroups[str.RepoRootDir(override)]
	if currentItemIsPartofGroup {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  host %s is part of group %s\n", current, group)
		skip = false
		return
	}

	// Split choices on comma
	userHostChoices := strings.SplitSeq(override, ",")

	// Check each override specified against current
	for userChoice := range userHostChoices {
		// Only assume override choice is regex if user requested it
		if opts.RegexEnabled {
			// Prepare user choice as regex
			userRegex, err := regexp.Compile(userChoice)
			if err != nil {
				// Invalid regex, always skip (but print high verbosity what happened)
				logctx.LogStdWarn(ctx, "Invalid regular expression: %w", err)
				return
			}

			// Check if user regex matches current item, if so return
			if userRegex.MatchString(current) {
				skip = false
				return
			}
		}

		// Don't skip if current is user choice
		if userChoice == current {
			skip = false
			return
		}
		skip = true
	}

	return
}
