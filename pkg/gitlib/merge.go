package gitlib

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type MergeCheckConflictedFileInfo struct {
	// object mode
	Mode int `json:"mode"`
	ObjectId string `json:"oid"`
	Stage int `json:"stage"`
	FileName string `json:"fileName"`
}
type MergeCheckInformationalMessage struct {
	Path []string `json:"path"`
	Type string `json:"type"`
	Message string `json:"msg"`
}
type MergeCheckResult struct {
	Successful bool `json:"success"`
	ReceiverLocation string `json:"receiverLocation"`
	ReceiverBranch string `json:"receiverBranch"`
	ProviderRemoteName string `json:"providerRemoteName"`
	ProviderBranch string `json:"providerBranch"`
	ToplevelTreeOid string `json:"rootOid"`
	FileInfo []MergeCheckConflictedFileInfo `json:"fileInfo"`
	Message []MergeCheckInformationalMessage `json:"msg"`
}

func parseMergeCheckZResult(s string) *MergeCheckResult {
	// get top level oid.
	ss := strings.SplitN(s, "\x00", 2)
	topOid := string(ss[0])
	if len(ss[1]) <= 0 {
		return &MergeCheckResult{
			Successful: true,
			ToplevelTreeOid: topOid,
			FileInfo: nil,
			Message: nil,
		}
	}
	subj := ss[1]
	// get conflicted file info.
	cfl := make([]MergeCheckConflictedFileInfo, 0)
	for {
		ss := strings.SplitN(subj, "\x00", 2)
		if ss[0] == "" { subj = ss[1]; break }
		ss1 := strings.Split(ss[0], "\t")
		fileName := ss1[1]
		ss2 := strings.Split(ss1[0], " ")
		fileMode, _ := strconv.Atoi(ss2[0])
		fileObjId := ss2[1]
		stage, _ := strconv.Atoi(ss2[2])
		cfl = append(cfl, MergeCheckConflictedFileInfo{
			Mode: fileMode,
			ObjectId: fileObjId,
			Stage: stage,
			FileName: fileName,
		})
		subj = ss[1]
	}
	// get informational message.
	msgl := make([]MergeCheckInformationalMessage, 0)
	for len(subj) > 0 {
		// get list of paths
		pathList := make([]string, 0)
		ss = strings.SplitN(subj, "\x00", 2)
		subj = ss[1]
		pathnum, _ := strconv.Atoi(ss[0])
		for range pathnum {
			ss = strings.SplitN(subj, "\x00", 2)
			subj = ss[1]
			pathList = append(pathList, ss[0])
		}
		ss = strings.SplitN(subj, "\x00", 2)
		conflictType := ss[0]
		subj = ss[1]
		ss = strings.SplitN(subj, "\x00", 2)
		conflictMessage := ss[0]
		subj = ss[1]
		msgl = append(msgl, MergeCheckInformationalMessage{
			Path: pathList,
			Type: conflictType,
			Message: conflictMessage,
		})
	}
	return &MergeCheckResult{
		Successful: false,
		ToplevelTreeOid: topOid,
		FileInfo: cfl,
		Message: msgl,
	}
}

func (gr LocalGitRepository) SetUpMergeTarget(providerName string, providerPath string) error {
	cmd := exec.Command("git", "remote", "add", providerName, providerPath)
	cmd.Dir = gr.GitDirectoryPath
	buf := new(bytes.Buffer)
	cmd.Stderr = buf
	err := cmd.Run()
	if err != nil {
		if strings.Contains(buf.String(), "already exists") { return nil }
		return err
	}
	return nil
}

func (gr LocalGitRepository) CheckBranchMergeConflict(localBranch string, remote string, remoteBranch string) (*MergeCheckResult, error) {
	// this would fetch the branch for you.
	cmd := exec.Command("git", "fetch", remote, remoteBranch)
	buf := new(bytes.Buffer)
	cmd.Stderr = buf
	cmd.Dir = gr.GitDirectoryPath
	err := cmd.Run()
	if err != nil {
		return nil, errors.New(err.Error() + ": " + buf.String())
	}
	remoteBranchFullName := fmt.Sprintf("%s/%s", remote, remoteBranch)
	cmd2 := exec.Command("git", "merge-tree", "-z", localBranch, remoteBranchFullName)
	buf.Reset()
	cmd2.Stdout = buf
	cmd2.Dir = gr.GitDirectoryPath
	err = cmd2.Run()
	// NOTE: we must check exit status because that's what the
	// document says:
	//   Do NOT interpret an empty Conflicted file info
	//   list as a clean merge; check the exit status. A merge can have
	//   conflicts without having individual files conflict (there are a
	//   few types of directory rename conflicts that fall into this
	//   category, and others might also be added in the future).
	// the command would not put things to stderr - everything is put
	// to stdout.
	treeId := strings.TrimSpace(buf.String())
	preres := parseMergeCheckZResult(treeId)
	if err != nil {
		preres.Successful = false
	} else {
		preres.Successful = true
	}
	preres.ReceiverLocation = gr.GitDirectoryPath
	preres.ReceiverBranch = localBranch
	preres.ProviderRemoteName = remote
	preres.ProviderBranch = remoteBranch
	return preres, nil
}

// Merge methods supported by Merge. "merge" creates a merge commit with two
// parents, "squash" creates a single commit out of the merged tree, and
// "rebase" replays the provider commits onto the receiver branch.
const (
	MERGE_METHOD_MERGE = "merge"
	MERGE_METHOD_SQUASH = "squash"
	MERGE_METHOD_REBASE = "rebase"
)

// ErrMergeConflict is returned when a merge cannot be performed because the
// branches conflict.
var ErrMergeConflict = errors.New("the pull request has merge conflicts")

// MergeMethods lists the methods in the order they are offered to users.
var MergeMethods = []string{
	MERGE_METHOD_MERGE,
	MERGE_METHOD_SQUASH,
	MERGE_METHOD_REBASE,
}

func ValidMergeMethod(method string) bool {
	for _, m := range MergeMethods {
		if m == method { return true }
	}
	return false
}

// emptyTreeOid is the object id of the empty tree. It is used as the merge
// base when replaying a root commit during a rebase.
const emptyTreeOid = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func runGitCommand(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if len(env) > 0 { cmd.Env = append(os.Environ(), env...) }
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 0 {
			return "", fmt.Errorf("git %s: %s: %s", args[0], err.Error(), msg)
		}
		return "", fmt.Errorf("git %s: %s", args[0], err.Error())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func commitIdentity(name string, email string) []string {
	return []string{
		fmt.Sprintf("GIT_AUTHOR_NAME=%s", name),
		fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", email),
		fmt.Sprintf("GIT_COMMITTER_NAME=%s", name),
		fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", email),
	}
}

// writeMergeTree merges provider into the commit-ish base and returns the id of
// the resulting tree. A non-zero exit status means the merge has conflicts.
func writeMergeTree(dir string, base string, provider string, mergeBase string) (string, error) {
	args := []string{"merge-tree", "--write-tree"}
	if len(mergeBase) > 0 { args = append(args, fmt.Sprintf("--merge-base=%s", mergeBase)) }
	args = append(args, base, provider)
	return runGitCommand(dir, nil, args...)
}

// mergeCommit creates a commit with both branches as parents.
func (gr LocalGitRepository) mergeCommit(localBranch string, providerFullName string, message string, name string, email string) (string, error) {
	treeId, err := writeMergeTree(gr.GitDirectoryPath, localBranch, providerFullName, "")
	if err != nil { return "", err }
	return runGitCommand(gr.GitDirectoryPath, commitIdentity(name, email),
		"commit-tree", treeId, "-m", message, "-p", localBranch, "-p", providerFullName)
}

// squashCommit creates a single commit whose tree is the merged result and
// whose only parent is the receiver branch.
func (gr LocalGitRepository) squashCommit(localBranch string, providerFullName string, message string, name string, email string) (string, error) {
	treeId, err := writeMergeTree(gr.GitDirectoryPath, localBranch, providerFullName, "")
	if err != nil { return "", err }
	return runGitCommand(gr.GitDirectoryPath, commitIdentity(name, email),
		"commit-tree", treeId, "-m", message, "-p", localBranch)
}

// isAncestor reports whether ancestor is reachable from descendant.
func (gr LocalGitRepository) isAncestor(ancestor string, descendant string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = gr.GitDirectoryPath
	return cmd.Run() == nil
}

// copyCommitAuthor returns the author identity and date of the commit, which
// a rebase keeps so the replayed commit still credits the original author.
func (gr LocalGitRepository) copyCommitAuthor(commitId string) ([]string, string, error) {
	out, err := runGitCommand(gr.GitDirectoryPath, nil, "show", "-s", "--format=%an%x00%ae%x00%aI%x00%B", commitId)
	if err != nil { return nil, "", err }
	parts := strings.SplitN(out, "\x00", 4)
	if len(parts) != 4 { return nil, "", fmt.Errorf("unexpected commit metadata for %s", commitId) }
	return []string{
		fmt.Sprintf("GIT_AUTHOR_NAME=%s", parts[0]),
		fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", parts[1]),
		fmt.Sprintf("GIT_AUTHOR_DATE=%s", parts[2]),
	}, parts[3], nil
}

// rebaseCommits replays the commits that are only reachable from the provider
// branch onto the receiver branch, one merge-tree/commit-tree pair per commit.
// Nothing is written to the ref until every commit has been replayed.
func (gr LocalGitRepository) rebaseCommits(localBranch string, providerFullName string, name string, email string) (string, error) {
	out, err := runGitCommand(gr.GitDirectoryPath, nil,
		"rev-list", "--reverse", "--no-merges", fmt.Sprintf("%s..%s", localBranch, providerFullName))
	if err != nil { return "", err }
	commits := make([]string, 0)
	if len(out) > 0 { commits = strings.Fields(out) }
	tip := localBranch
	for _, commitId := range commits {
		tipTree, err := runGitCommand(gr.GitDirectoryPath, nil, "rev-parse", fmt.Sprintf("%s^{tree}", tip))
		if err != nil { return "", err }
		mergeBase, err := runGitCommand(gr.GitDirectoryPath, nil, "rev-parse", fmt.Sprintf("%s^", commitId))
		if err != nil { mergeBase = emptyTreeOid }
		treeId, err := writeMergeTree(gr.GitDirectoryPath, tip, commitId, mergeBase)
		if err != nil {
			short, _ := runGitCommand(gr.GitDirectoryPath, nil, "rev-parse", "--short", commitId)
			return "", fmt.Errorf("cannot rebase commit %s: %s", short, err.Error())
		}
		// A commit that no longer changes anything is dropped, like git-rebase
		// does by default.
		if treeId == tipTree { continue }
		authorEnv, message, err := gr.copyCommitAuthor(commitId)
		if err != nil { return "", err }
		env := append(authorEnv,
			fmt.Sprintf("GIT_COMMITTER_NAME=%s", name),
			fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", email),
		)
		tip, err = runGitCommand(gr.GitDirectoryPath, env, "commit-tree", treeId, "-m", message, "-p", tip)
		if err != nil { return "", err }
	}
	return tip, nil
}

// Merge merges remoteBranch of remote into localBranch and moves the local
// branch to the result. message is used as the subject of the squashed commit;
// it is ignored by the other methods.
func (gr LocalGitRepository) Merge(remote string, remoteBranch string, localBranch string, method string, name string, email string, message string) error {
	if !ValidMergeMethod(method) { return fmt.Errorf("unknown merge method: %s", method) }
	_, err := runGitCommand(gr.GitDirectoryPath, nil, "fetch", "--", remote, remoteBranch)
	if err != nil { return err }
	providerFullName := fmt.Sprintf("%s/%s", remote, remoteBranch)
	if len(message) <= 0 {
		message = fmt.Sprintf("%s: from %s to %s", method, providerFullName, localBranch)
	}
	var commitId string
	switch method {
	case MERGE_METHOD_MERGE:
		commitId, err = gr.mergeCommit(localBranch, providerFullName,
			fmt.Sprintf("merge: from %s to %s", providerFullName, localBranch), name, email)
	case MERGE_METHOD_SQUASH:
		commitId, err = gr.squashCommit(localBranch, providerFullName, message, name, email)
	case MERGE_METHOD_REBASE:
		// a fork that is simply ahead is fast-forwarded instead of being
		// replayed commit by commit.
		if gr.isAncestor(localBranch, providerFullName) {
			commitId, err = runGitCommand(gr.GitDirectoryPath, nil, "rev-parse", providerFullName)
		} else {
			commitId, err = gr.rebaseCommits(localBranch, providerFullName, name, email)
		}
	}
	if err != nil { return err }
	if len(commitId) <= 0 { return fmt.Errorf("merge produced no commit") }
	_, err = runGitCommand(gr.GitDirectoryPath, nil,
		"update-ref", fmt.Sprintf("refs/heads/%s", localBranch), commitId)
	return err
}




































































