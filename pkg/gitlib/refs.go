package gitlib

import (
	"fmt"
	"os"
	"path"
	"strings"
)

// HeadBranch returns the local branch that HEAD points at. A detached HEAD
// returns an empty string.
func (gr LocalGitRepository) HeadBranch() (string, error) {
	b, err := os.ReadFile(path.Join(gr.GitDirectoryPath, "HEAD"))
	if err != nil { return "", err }
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "ref: refs/heads/") { return "", nil }
	return strings.TrimPrefix(s, "ref: refs/heads/"), nil
}

// refNameProblem returns a description of why name cannot be used as a ref,
// or an empty string when it can. name is the short name, i.e. without
// refs/heads/ or refs/tags/.
func (gr LocalGitRepository) refNameProblem(fullName string, name string) string {
	// check-ref-format accepts a leading dash, but a name like "-x" is
	// unusable on the command line.
	if strings.HasPrefix(name, "-") {
		return fmt.Sprintf("%q is not a valid name", name)
	}
	if _, err := runGitCommand(gr.GitDirectoryPath, nil, "check-ref-format", fullName); err != nil {
		return fmt.Sprintf("%q is not a valid name", name)
	}
	return ""
}

// CreateBranch points refs/heads/name at target, which is any commit-ish
// (a branch, a tag or a commit id). It refuses to overwrite an existing
// branch.
func (gr LocalGitRepository) CreateBranch(name string, target string) error {
	return gr.createRef("refs/heads/" + name, "branch", name, target)
}

// DeleteBranch deletes refs/heads/name. It refuses to delete the branch HEAD
// points at, which would leave the repository without a default branch.
func (gr LocalGitRepository) DeleteBranch(name string) error {
	head, err := gr.HeadBranch()
	if err != nil { return err }
	if head == name {
		return fmt.Errorf("branch %q is the branch HEAD points at and cannot be deleted", name)
	}
	return gr.deleteRef("refs/heads/" + name, "branch", name)
}

// CreateTag points refs/tags/name at target, which is any commit-ish. It
// creates a lightweight tag and refuses to overwrite an existing tag.
func (gr LocalGitRepository) CreateTag(name string, target string) error {
	return gr.createRef("refs/tags/" + name, "tag", name, target)
}

// DeleteTag deletes refs/tags/name.
func (gr LocalGitRepository) DeleteTag(name string) error {
	return gr.deleteRef("refs/tags/" + name, "tag", name)
}

func (gr LocalGitRepository) createRef(fullName string, kind string, name string, target string) error {
	if problem := gr.refNameProblem(fullName, name); len(problem) > 0 {
		return fmt.Errorf("%s %s", kind, problem)
	}
	if gr.refExists(fullName) {
		return fmt.Errorf("%s %q already exists", kind, name)
	}
	commitId, err := runGitCommand(gr.GitDirectoryPath, nil, "rev-parse", "--verify", "--quiet", target+"^{commit}")
	if err != nil || len(commitId) <= 0 {
		return fmt.Errorf("%s %q cannot be created: %q does not resolve to a commit", kind, name, target)
	}
	// The empty old value makes git refuse the update if the ref appears
	// between the check above and this call.
	_, err = runGitCommand(gr.GitDirectoryPath, nil, "update-ref", fullName, commitId, "")
	if err != nil {
		return fmt.Errorf("%s %q cannot be created: %s", kind, name, err.Error())
	}
	return nil
}

func (gr LocalGitRepository) deleteRef(fullName string, kind string, name string) error {
	if !gr.refExists(fullName) {
		return fmt.Errorf("%s %q does not exist", kind, name)
	}
	_, err := runGitCommand(gr.GitDirectoryPath, nil, "update-ref", "-d", fullName)
	if err != nil {
		return fmt.Errorf("%s %q cannot be deleted: %s", kind, name, err.Error())
	}
	return nil
}

// ResolveBranchTip returns the commit id of a local branch. The branch list is
// refreshed first, so the result is the current tip even when the repository
// handle was created before the push.
func (gr *LocalGitRepository) ResolveBranchTip(branch string) (string, error) {
	if err := gr.SyncBranch(branch); err != nil { return "", err }
	if gr.BranchIndex == nil { return "", ErrObjectNotFound }
	b, ok := gr.BranchIndex[branch]
	if !ok || len(b.HeadId) <= 0 { return "", ErrObjectNotFound }
	return b.HeadId, nil
}

// ResolveRemoteBranchTip returns the commit id of a remote-tracking branch,
// e.g. refs/remotes/ns:repo/branch. It is used after a fetch, when the
// receiver needs to know exactly which provider commit it is about to merge.
func (gr LocalGitRepository) ResolveRemoteBranchTip(remoteName string, branch string) (string, error) {
	return runGitCommand(gr.GitDirectoryPath, nil,
		"rev-parse", "--verify", fmt.Sprintf("refs/remotes/%s/%s", remoteName, branch))
}
