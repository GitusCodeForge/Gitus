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

func (gr LocalGitRepository) Merge(remote string, remoteBranch string, localBranch string, author string, email string) error {
	buf := new(bytes.Buffer)
	cmd1 := exec.Command("git", "fetch", remote, remoteBranch)
	cmd1.Dir = gr.GitDirectoryPath
	cmd1.Stderr = buf
	err := cmd1.Run()
	if err != nil { return fmt.Errorf("%s: %s", err.Error(), buf.String()) }
	buf.Reset()
	providerFullName := fmt.Sprintf("%s/%s", remote, remoteBranch)
	cmd2 := exec.Command("git", "merge-tree", "--write-tree", localBranch, providerFullName)
	cmd2.Dir = gr.GitDirectoryPath
	cmd2.Stdout = buf
	err = cmd2.Run()
	if err != nil { return fmt.Errorf("Failed while merge-tree: %s", err.Error()) }
	treeId := strings.TrimSpace(buf.String())
	mergeMessage := fmt.Sprintf("merge: from %s/%s to %s", remote, remoteBranch, localBranch)
	buf.Reset()
	cmd3 := exec.Command("git", "commit-tree", treeId, "-m", mergeMessage, "-p", localBranch, "-p", providerFullName)
	cmd3.Dir = gr.GitDirectoryPath
	cmd3.Env = os.Environ()
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_AUTHOR_NAME=%s", author))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", email))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_COMMITTER_NAME=%s", author))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", email))
	err = cmd3.Run()
	if err != nil { return fmt.Errorf("Failed while commit-tree: %s", err.Error()) }
	commitId := strings.TrimSpace(buf.String())
	buf.Reset()
	localBranchFullName := fmt.Sprintf("refs/heads/%s", localBranch)
	cmd4 := exec.Command("git", "update-ref", localBranchFullName, commitId)
	cmd4.Dir = gr.GitDirectoryPath
	cmd4.Stderr = buf
	err = cmd4.Run()
	if err != nil { return fmt.Errorf("Failed while update-ref: %s; %s", err.Error(), buf.String()) }
	return nil
}




































































