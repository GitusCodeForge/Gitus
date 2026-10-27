package gitlib

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

var HookList = []string{
	"applypatch-msg",
	"pre-applypatch",
	"post-applypatch",
	"pre-commit",
	"pre-merge-commit",
	"prepare-commit-msg",
	"commit-msg",
	"post-commit",
	"pre-rebase",
	"post-checkout",
	"post-merge",
	"pre-push",
	"pre-receive",
	"update",
	"proc-receive",
	"post-receive",
	"post-update",
	"reference-transaction",
	"push-to-checkout",
	"pre-auto-gc",
	"post-rewrite",
	"rebase",
	"sendemail-validate",
	"fsmonitor-watchman",
	"p4-changelist",
	"p4-prepare-changelist",
	"p4-post-changelist",
	"p4-pre-submit",
	"post-index-change",
}

func (lgr LocalGitRepository) GetAllSetHooksName() ([]string, error) {
	res := make([]string, 0)
	for _, item := range HookList {
		p := path.Join(lgr.GitDirectoryPath, "hooks", item)
		f, err := os.Open(p)
		if err != nil { continue }
		res = append(res, item)
		f.Close()
	}
	return res, nil
}

func (lgr LocalGitRepository) GetHook(hookName string) (string, error) {
	p := path.Join(lgr.GitDirectoryPath, "hooks", hookName)
	s, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) { return "", nil }
		return "", err
	}
	return string(s), nil
}

func (lgr LocalGitRepository) SaveHook(hookName string, hookContent string) error {
	p := path.Join(lgr.GitDirectoryPath, "hooks", hookName)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0764)
	if err != nil { return err }
	defer f.Close()
	_, err = f.WriteString(hookContent)
	if err != nil { return err }
	return nil
}

func (lgr LocalGitRepository) DeleteHook(hookName string) error {
	if lgr.Hooks == nil { lgr.Hooks = make(map[string]string, 0) }
	delete(lgr.Hooks, hookName)
	p := path.Join(lgr.GitDirectoryPath, "hooks", hookName)
	return os.Remove(p)
}


// Hooks that Gitus installs carry this line. A hook that does not carry it is
// considered to be somebody else's and is never overwritten.
const managedHookMarker = "# managed by gitus"

// ErrHookNotManaged is returned when the repository already has an update hook
// that was not written by Gitus.
var ErrHookNotManaged = errors.New("the update hook of this repository was not installed by gitus")

// ManagedHookParams is everything the generated hook needs to call back into
// Gitus. The hook runs with the environment of git-receive-pack, so both paths
// have to be absolute and the repository name has to be baked in.
type ManagedHookParams struct {
	ConfigPath string
	RepoFullName string
}

const managedUpdateHookTemplate = `#!/bin/sh
%s
#
# Reinstalled by gitus whenever the repository is created or
# ` + "`gitus install-hooks`" + ` is run; do not edit.
#
# git runs this for every ref that a push updates (see githooks(5)). It only
# notifies: it sends the webhook configured for this repository and warms up
# the "last commit" listing of the new commit. Neither may reject the push, so
# everything is best effort and the exit code is always 0.

if [ -z "$GIT_DIR" ]; then
	printf 'This hook is meant to be run by git.\n' >&2
	exit 0
fi

refname="$1"
oldrev="$2"
newrev="$3"

case "$newrev" in
	*[!0]*) newrev_type=$(git cat-file -t "$newrev" 2>/dev/null) ;;
	*) newrev_type=delete ;;
esac
if [ -z "$newrev_type" ]; then
	newrev_type=commit
fi

%s -config %s update-trigger %s "$refname" "$newrev_type" "$oldrev" "$newrev" >/dev/null 2>&1 || true

exit 0
`

func (gr *LocalGitRepository) updateHookPath() string {
	return path.Join(gr.GitDirectoryPath, "hooks", "update")
}

// isGitusHook reports whether content is a hook Gitus owns: either the managed
// one, or the older webhook hook that was written before Gitus managed hooks
// centrally. The latter called `gitus web-hooks send` without a path or config,
// so it is replaced rather than kept.
func isGitusHook(content string) bool {
	return strings.Contains(content, managedHookMarker) ||
		strings.Contains(content, "gitus web-hooks send")
}

// HasManagedHook reports whether the update hook of the repository is one that
// Gitus installed.
func (gr *LocalGitRepository) HasManagedHook() bool {
	b, err := os.ReadFile(gr.updateHookPath())
	if err != nil { return false }
	return isGitusHook(string(b))
}

// InstallManagedHook writes (or refreshes) the update hook of the repository.
// It refuses to touch a hook that Gitus did not write, and does nothing when the
// file is already up to date.
func (gr *LocalGitRepository) InstallManagedHook(p ManagedHookParams) error {
	if len(p.ConfigPath) <= 0 {
		return errors.New("the configuration file path is unknown; cannot generate the hook")
	}
	exe, err := os.Executable()
	if err != nil { return err }
	content := fmt.Sprintf(managedUpdateHookTemplate,
		managedHookMarker,
		shellQuote(exe), shellQuote(p.ConfigPath), shellQuote(p.RepoFullName))

	hookPath := gr.updateHookPath()
	existing, err := os.ReadFile(hookPath)
	if err == nil {
		if !isGitusHook(string(existing)) {
			return ErrHookNotManaged
		}
		if string(existing) == content {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	err = os.MkdirAll(path.Dir(hookPath), 0755)
	if err != nil { return err }
	// Written aside and renamed into place: a half written hook would be run by
	// the next push and could reject it.
	f, err := os.CreateTemp(path.Dir(hookPath), ".update-*")
	if err != nil { return err }
	tmpName := f.Name()
	defer os.Remove(tmpName)
	_, err = f.WriteString(content)
	if err != nil { f.Close(); return err }
	err = f.Close()
	if err != nil { return err }
	err = os.Chmod(tmpName, 0755)
	if err != nil { return err }
	return os.Rename(tmpName, hookPath)
}

// shellQuote quotes s for /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
