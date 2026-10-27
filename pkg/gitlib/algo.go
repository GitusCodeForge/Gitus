package gitlib

import (
	"bytes"
	"os/exec"
)

// ResolvePathLastCommitId returns the id of the newest commit reachable from
// cobj that changed the path p.
//
// This spawns one git process for one path; when a whole directory listing is
// needed use ResolveLastCommitAtDir instead, which resolves every entry with a
// single walk of the history.
func (gr LocalGitRepository) ResolvePathLastCommitId(cobj *CommitObject, p string) (string, error) {
	cmd := exec.Command("git", "rev-list", "-1", cobj.Id, "--", p)
	cmd.Dir = gr.GitDirectoryPath
	buf := new(bytes.Buffer)
	cmd.Stdout = buf
	err := cmd.Run()
	if err != nil { return "", err }
	return buf.String(), nil
}
