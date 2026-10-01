package gitlib

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"strings"
)

// a wrapper over git-rev-list.
// TODO: find a better way to do this...
func (gr LocalGitRepository) ResolvePathLastCommitId(cobj *CommitObject, p string) (string, error) {
	cmd := exec.Command("git", "rev-list", "-1", cobj.Id, "--", p)
	cmd.Dir = gr.GitDirectoryPath
	buf := new(bytes.Buffer)
	cmd.Stdout = buf
	err := cmd.Run()
	if err != nil { return "", err }
	return buf.String(), nil
}

func (gr LocalGitRepository) ResolveLastCommitAtPath(cobj *CommitObject, p string) (map[string]string, error) {
	cmd := exec.Command("git", "log", "--name-only", "--format=tformat:%H", cobj.Id, "--", p)
	cmd.Dir = gr.GitDirectoryPath
	buf := new(bytes.Buffer)
	cmd.Stdout = buf
	err := cmd.Run()
	if err != nil { return nil, err }
	res := make(map[string]string, 0)
	lr := newTrueLineReader(buf)
	currentCommit := ""
	for {
		l, err := lr.readLine()
		if errors.Is(err, io.EOF) { break }
		if err != nil { return nil, err }
		l = strings.TrimSpace(l)
		if IsValidId(l) {
			currentCommit = l
		} else if l == "" {
			continue
		} else {
			_, ok := res[l]
			if ok { continue }
			res[l] = currentCommit
		}
	}
	return res, nil
}



