package gitlib

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var REGEX_VALID_BRANCH_NAME = regexp.MustCompile("^[A-Za-z0-9._/-]+$")
func validBranchName(s string) bool {
	return REGEX_VALID_BRANCH_NAME.MatchString(s)
}

var REGEX_VALID_FILE_PATH = regexp.MustCompile("^[A-Za-z0-9._/-]+$")
func validFilePath(s string) bool {
	return REGEX_VALID_FILE_PATH.MatchString(s)
}

func (gr *LocalGitRepository) AddFileToRepoString(
	branchName string, filePath string,
	authorName string, authorEmail string,
	committerName string, committerEmail string,
	commitMessage string,
	content string,
) (string, error) {
	if !validBranchName(branchName) {
		return "", errors.New("Invalid branch name")
	}
	if strings.Contains(filePath, "\n") || !filepath.IsLocal(filepath.Clean(filePath)) {
		return "", errors.New("Invalid file path")
	}
	if strings.Contains(authorName, "\n") {
		return "", errors.New("Invalid author name")
	}
	if strings.Contains(authorEmail, "\n") {
		return "", errors.New("Invalid author email")
	}
	if strings.Contains(committerName, "\n") {
		return "", errors.New("Invalid committer name")
	}
	if strings.Contains(committerEmail, "\n") {
		return "", errors.New("Invalid committer email")
	}
	err := gr.SyncBranch(branchName)
	if err != nil { return "", err }
	_, ok := gr.BranchIndex[branchName]
	var fromStr string
	if !ok {
		fromStr = ""
	} else {
		fromStr = fmt.Sprintf("from refs/heads/%s^0\n", branchName)
	}
	cmd := exec.Command("git", "fast-import", "--date-format=now", "--quiet")
	cmd.Dir = gr.GitDirectoryPath
	stdoutBuff := new(bytes.Buffer)
	cmd.Stdout = stdoutBuff
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	stdinPipe, err := cmd.StdinPipe()
	if err != nil { return "", err }
	payload := fmt.Sprintf(`commit refs/heads/%s
mark :1
author %s <%s> now
committer %s <%s> now
data %d
%s
%sM 100644 inline %s
data %d
%s
get-mark :1`,
		branchName, authorName, authorEmail,
		committerName, committerEmail,
		len(commitMessage), commitMessage,
		fromStr,
		filePath,
		len(content), content,
	)
	err = cmd.Start()
	if err != nil { return "", err }
	_, err = stdinPipe.Write([]byte(payload))
	if err != nil { return "", err }
	err = stdinPipe.Close()
	if err != nil { return "", err }
	err = cmd.Wait()
	if err != nil { return "", fmt.Errorf("%s; %s", err, stderrBuf.String()) }
	newestCommitId := strings.TrimSpace(stdoutBuff.String())
	if gr.BranchIndex == nil {
		gr.BranchIndex = make(map[string]*Branch, 0)
	}
	k, ok := gr.BranchIndex[branchName]
	if !ok {
		gr.BranchIndex[branchName] = &Branch{
			Name: branchName,
			HeadId: newestCommitId,
		}
	} else {
		k.HeadId = newestCommitId
	}
	return newestCommitId, nil
}

func (gr *LocalGitRepository) AddFileToRepoReader(
	branchName string, treePath string,
	authorName string, authorEmail string,
	committerName string, committerEmail string,
	commitMessage string,
	content io.Reader, contentSize int64,
) (string, error) {
	if !validBranchName(branchName) {
		return "", errors.New("Invalid branch name")
	}
	if strings.Contains(treePath, "\n") || !filepath.IsLocal(filepath.Clean(treePath)) {
		return "", errors.New("Invalid file path")
	}
	if strings.Contains(authorName, "\n") {
		return "", errors.New("Invalid author name")
	}
	if strings.Contains(authorEmail, "\n") {
		return "", errors.New("Invalid author email")
	}
	if strings.Contains(committerName, "\n") {
		return "", errors.New("Invalid committer name")
	}
	if strings.Contains(committerEmail, "\n") {
		return "", errors.New("Invalid committer email")
	}
	err := gr.SyncBranch(branchName)
	if err != nil { return "", err }
	_, ok := gr.BranchIndex[branchName]
	var fromStr string
	if !ok {
		fromStr = ""
	} else {
		fromStr = fmt.Sprintf("from refs/heads/%s^0\n", branchName)
	}
	cmd := exec.Command("git", "fast-import", "--date-format=now", "--quiet")
	cmd.Dir = gr.GitDirectoryPath
	stdoutBuff := new(bytes.Buffer)
	cmd.Stdout = stdoutBuff
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	stdinPipe, err := cmd.StdinPipe()
	if err != nil { return "", err }
	payload := fmt.Sprintf(`commit refs/heads/%s
mark :1
author %s <%s> now
committer %s <%s> now
data %d
%s
%sM 100644 inline %s
data %d
`,
		branchName, authorName, authorEmail,
		committerName, committerEmail,
		len(commitMessage), commitMessage,
		fromStr,
		treePath,
		contentSize,
	)
	err = cmd.Start()
	if err != nil { return "", err }
	_, err = stdinPipe.Write([]byte(payload))
	if err != nil { return "", err }
	_, err = io.Copy(stdinPipe, content)
	if err != nil { return "", err }
	_, err = stdinPipe.Write([]byte(`
get-mark :1`))
	if err != nil { return "", err }
	err = stdinPipe.Close()
	if err != nil { return "", err }
	err = cmd.Wait()
	if err != nil { return "", err }
	newestCommitId := stdoutBuff.String()
	if gr.BranchIndex == nil {
		gr.BranchIndex = make(map[string]*Branch, 0)
	}
	k, ok := gr.BranchIndex[branchName]
	if !ok {
		gr.BranchIndex[branchName] = &Branch{
			Name: branchName,
			HeadId: newestCommitId,
		}
	} else {
		k.HeadId = newestCommitId
	}
	return newestCommitId, nil
}

func (gr *LocalGitRepository) AddMultipleFileToRepoString(
	branchName string,
	authorName string, authorEmail string,
	committerName string, committerEmail string,
	commitMessage string,
	content map[string]string,
) (string, error){
	if !validBranchName(branchName) {
		return "", errors.New("Invalid branch name")
	}
	for k, _ := range content {
		if strings.Contains(k, "\n") || !filepath.IsLocal(filepath.Clean(k)) {
			return "", errors.New("Invalid file path")
		}
	}
	if strings.Contains(authorName, "\n") {
		return "", errors.New("Invalid author name")
	}
	if strings.Contains(authorEmail, "\n") {
		return "", errors.New("Invalid author email")
	}
	if strings.Contains(committerName, "\n") {
		return "", errors.New("Invalid committer name")
	}
	if strings.Contains(committerEmail, "\n") {
		return "", errors.New("Invalid committer email")
	}
	err := gr.SyncBranch(branchName)
	if err != nil { return "", err }
	_, ok := gr.BranchIndex[branchName]
	var fromStr string
	if !ok {
		fromStr = ""
	} else {
		fromStr = fmt.Sprintf("from refs/heads/%s^0\n", branchName)
	}
	cmd := exec.Command("git", "fast-import", "--date-format=now", "--quiet")
	cmd.Dir = gr.GitDirectoryPath
	stdoutBuff := new(bytes.Buffer)
	cmd.Stdout = stdoutBuff
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	stdinPipe, err := cmd.StdinPipe()
	if err != nil { return "", err }
	payloadBuffer := new(bytes.Buffer)
	fmt.Fprintf(payloadBuffer, `commit refs/heads/%s
mark :1
author %s <%s> now
committer %s <%s> now
data %d
%s
%s`,
		branchName, authorName, authorEmail,
		committerName, committerEmail,
		len(commitMessage), commitMessage,
		fromStr,
	)
	for k, v := range content {
		fmt.Fprintf(payloadBuffer, `M 100644 inline %s
data %d
%s
`, k, len(v), v)
	}
	fmt.Fprint(payloadBuffer, `get-mark :1`)
	err = cmd.Start()
	if err != nil { return "", fmt.Errorf("%s: %s", err, stderrBuf.String()) }
	_, err = stdinPipe.Write(payloadBuffer.Bytes())
	if err != nil { return "", fmt.Errorf("%s: %s", err, stderrBuf.String()) }
	err = stdinPipe.Close()
	if err != nil { return "", fmt.Errorf("%s: %s", err, stderrBuf.String()) }
	err = cmd.Wait()
	if err != nil { return "", fmt.Errorf("%s: %s", err, stderrBuf.String()) }
	newestCommitId := strings.TrimSpace(stdoutBuff.String())
	if gr.BranchIndex == nil {
		gr.BranchIndex = make(map[string]*Branch, 0)
	}
	k, ok := gr.BranchIndex[branchName]
	if !ok {
		gr.BranchIndex[branchName] = &Branch{
			Name: branchName,
			HeadId: newestCommitId,
		}
	} else {
		k.HeadId = newestCommitId
	}
	return newestCommitId, nil
}

