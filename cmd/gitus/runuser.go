package main

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
)

// allowRootEnv is the environment-variable form of -allow-root. It exists for
// container images that have no non-root user to switch to.
const allowRootEnv = "GITUS_ALLOW_ROOT"

// getEuid is a variable so the policy can be tested without actually running
// the test binary as root.
var getEuid = os.Geteuid

// runUserPolicy is what a subcommand does when it is started as root:
//
//   - the web server and the commands that run inside a repository or a git
//     hook refuse to start;
//   - reset-admin only warns, because it changes the database but does not
//     touch repositories;
//   - install and no-login are allowed: the web installer creates the git
//     user and fixes ownership, and no-login only prints a message.
type runUserPolicy struct {
	refuseRoot bool
	warnRoot   bool
}

func policyForCommand(command string) runUserPolicy {
	switch command {
	case "install", "no-login":
		return runUserPolicy{}
	case "reset-admin":
		return runUserPolicy{warnRoot: true}
	default:
		return runUserPolicy{refuseRoot: true}
	}
}

func allowRootFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(allowRootEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func commandLabel(command string) string {
	switch command {
	case "web-server":
		return "the web server"
	case "ssh":
		return "the ssh handler"
	case "web-hooks":
		return "the webhook sender"
	case "update-trigger":
		return "the update hook"
	case "storage-check":
		return "the storage check"
	case "host-mode":
		return "host-mode"
	case "install-hooks":
		return "install-hooks"
	}
	return fmt.Sprintf("%q", command)
}

// checkRunUser refuses the commands that must not run as root, and warns when
// the process user is not the configured git user. The latter is only a
// warning: browse-only mode does not use the git user for writing, and an
// administrator may have a reason for the mismatch.
func checkRunUser(cfg *gitus.GitusConfig, command string, allowRoot bool) error {
	policy := policyForCommand(command)
	euid := getEuid()

	if euid == 0 {
		if policy.refuseRoot && !allowRoot && !allowRootFromEnv() {
			return fmt.Errorf(
				"refusing to run %s as root; start Gitus as the %s user instead (use -allow-root or %s=1 only in a container)",
				commandLabel(command), gitUserName(cfg), allowRootEnv,
			)
		}
		if policy.refuseRoot || policy.warnRoot {
			log.Warnf("Running %s as root is not recommended; files it creates may be owned by root.", commandLabel(command))
		}
		return nil
	}

	gitUser := strings.TrimSpace(cfg.GitUser)
	if gitUser == "" {
		return nil
	}
	u, err := user.Lookup(gitUser)
	if err != nil {
		// The caller reports a clearer error when it needs this user.
		return nil
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil || uid == euid {
		return nil
	}
	log.Warnf(
		"Running as uid %d but gitUser %q is uid %d; repositories and hooks may be created with the wrong owner.",
		euid, gitUser, uid,
	)
	return nil
}

func gitUserName(cfg *gitus.GitusConfig) string {
	if name := strings.TrimSpace(cfg.GitUser); name != "" {
		return strconv.Quote(name)
	}
	return "configured git"
}
