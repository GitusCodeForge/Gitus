package gitlib

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/ini"
)

type LocalGitRepository struct {
	GitDirectoryPath string
	Description string
	PackIndex map[string]*PackIndex
	BranchIndex map[string]*Branch
	TagIndex map[string]*Tag
	Config ini.INI
	isSHA256 bool
	Hooks map[string]string
	Submodule map[string]*SubmoduleConfig
}

func (gr LocalGitRepository) IsSHA256() bool {
	return gr.isSHA256
}

func NewLocalGitRepository(p string) *LocalGitRepository {
	res := LocalGitRepository{
		GitDirectoryPath: p,
		PackIndex: nil,
		Hooks: nil,
	}
	pi, err := res.readAllPackIndex()
	if err != nil {
		errs := err.Error()
		os.MkdirAll(p, os.ModeDir|0755)
		cmd := exec.Command("git", "init", "--bare")
		cmd.Dir = p
		err := cmd.Run()
		if err != nil {
			log.Panicf("Failed to create a handle on local git repository:\n%s\n%s", errs, err.Error())
		} else {
			pi, err = res.readAllPackIndex()
		}
	}
	res.PackIndex = pi
	description, err := res.readDescription()
	if err != nil { description = "Error due to: " + err.Error() }
	res.Description = description
	config, err := res.readConfig()
	if err != nil {
		res.Config = nil
	} else {
		res.Config = config
	}
	if config == nil {
		res.isSHA256 = false
	} else {
		version, ok1 := config.GetValue("core", "", "repositoryformatversion")
		format, ok2 := config.GetValue("extensions", "", "objectformat")
		if ok1 && ok2 && version == "1" && strings.ToLower(format) == "sha256" {
			res.isSHA256 = true
		} else {
			res.isSHA256 = false
		}
	}
	res.LoadSubmoduleConfig()
	return &res
}

// UpdateServerInfo refreshes the files that the v1-dumb HTTP transport serves
// (info/refs, info/packs, ...). It used to run in NewLocalGitRepository, which
// meant one git process per repository handle - a namespace listing alone would
// spawn one per repository. It is only needed when somebody actually clones over
// the dumb protocol, so the clone handler calls it lazily.
func (gr LocalGitRepository) UpdateServerInfo() error {
	cmd := exec.Command("git", "update-server-info")
	cmd.Dir = gr.GitDirectoryPath
	return cmd.Run()
}

func (gr LocalGitRepository) SyncLocalDescription() error {
	descFilePath := path.Join(gr.GitDirectoryPath, "description")
	os.Remove(descFilePath)
	f, err := os.OpenFile(descFilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0664)
	if err != nil { return err }
	defer f.Close()
	f.Write([]byte(gr.Description))
	return nil
}

func (gr LocalGitRepository) readConfig() (ini.INI, error) {
	configFilePath := path.Join(gr.GitDirectoryPath, "config")
	f, err := os.Open(configFilePath)
	if err != nil { return nil, err }
	defer f.Close()
	return ini.ParseINI(f)
}

func (gr LocalGitRepository) containsRemote(s string) (bool, error) {
	cfg, err := gr.readConfig()
	if err != nil { return false, err }
	d, ok := cfg.GetSectionList("remote")
	if !ok { return false, nil }
	_, ok = d[s]
	return ok, nil
}

func (gr LocalGitRepository) readDescription() (string, error) {
	descriptionFilePath := path.Join(gr.GitDirectoryPath, "description")
	f, err := os.Open(descriptionFilePath)
	if err != nil { return "", err }
	defer f.Close()
	s, err := io.ReadAll(f)
	if err != nil { return "", err }
	return string(s), nil
}

func (gr LocalGitRepository) OpenDirectlyAccessibleObject(h string) (io.ReadCloser, error) {
	objStorePath := path.Join(gr.GitDirectoryPath, "objects")
	objPath := path.Join(objStorePath, h[:2], h[2:])
	f, err := os.Open(objPath)
	if err != nil { return nil, err }
	return zlib.NewReader(f)
}

// targetAbsDir must be absolute path.
func (gr LocalGitRepository) LocalForkTo(targetName string, targetAbsDir string) error {
	cmd := exec.Command("git", "clone", "--bare", gr.GitDirectoryPath, targetAbsDir)
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	err := cmd.Run()
	if err != nil {
		return errors.New(err.Error() + ": " + stderrBuf.String())
	}
	var cmd2 *exec.Cmd
	containsRemote, err := gr.containsRemote(targetName)
	if err != nil {
		return errors.New(err.Error() + ": " + stderrBuf.String())
	}
	if !containsRemote {
		cmd2 = exec.Command("git", "remote", "add", "--", targetName, targetAbsDir)
	} else {
		cmd2 = exec.Command("git", "remote", "set-url", "--", targetName, targetAbsDir)
	}
	stderrBuf.Reset()
	cmd2.Stderr = stderrBuf
	cmd2.Dir = gr.GitDirectoryPath
	err = cmd2.Run()
	if err != nil {
		return errors.New(err.Error() + ": " + stderrBuf.String())
	}
	// Populate the origin's remote-tracking refs for the new fork once, so the
	// fork page can compare without fetching on every view. Best effort: an
	// unfetched fork just has no comparison until its next push refreshes it.
	cmd3 := exec.Command("git", "fetch", "--", targetName)
	cmd3.Dir = gr.GitDirectoryPath
	cmd3.Run()
	return nil
}

func (gr LocalGitRepository) GetAllRemote() ([]string, error) {
	cfg, err := gr.readConfig()
	if err != nil { return nil, err }
	l, b := cfg.GetSectionList("remote")
	if !b { return []string{}, nil }
	res := make([]string, 0)
	for k := range l {
		res = append(res, k)
	}
	return res, nil
}

func (gr LocalGitRepository) HasRemote(s string) (bool, error) {
	cfg, err := gr.readConfig()
	if err != nil { return false, err }
	l, b := cfg.GetSectionList("remote")
	if !b { return false, nil }
	_, ok := l[s]
	return ok, nil
}

type BranchComparisonInfo struct {
	BaseId string
	ARevList []string
	BRevList []string
}

// NOTE: localBranch should *NOT* be full name.
//
// This runs on every repository page of a fork, so it must not fetch: it
// compares against the remote-tracking ref that the update hook refreshes when
// the fork is pushed. If that ref does not exist yet (a fork created before the
// hook did this, or a branch that was never pushed) there is nothing to
// compare, and the caller gets nil.
func (gr LocalGitRepository) CompareBranchWithRemote(localBranch string, remoteName string) (*BranchComparisonInfo, error) {
	hasRemote, err := gr.HasRemote(remoteName)
	if err != nil { return nil, err }
	if !hasRemote { return nil, nil }
	if !gr.refExists(fmt.Sprintf("refs/remotes/%s/%s", remoteName, localBranch)) {
		return nil, nil
	}
	stderrBuf := new(bytes.Buffer)
	cmd2 := exec.Command("git", "merge-base", fmt.Sprintf("refs/heads/%s", localBranch), fmt.Sprintf("%s/%s", remoteName, localBranch))
	cmd2.Dir = gr.GitDirectoryPath
	stdoutBuf := new(bytes.Buffer)
	cmd2.Stdout = stdoutBuf
	stderrBuf.Reset()
	cmd2.Stderr = stderrBuf
	err = cmd2.Run()
	if err != nil {
		return nil, fmt.Errorf("Failed to git-merge-base: %s; %s", err, stderrBuf.String())
	}
	baseId := strings.TrimSpace(stdoutBuf.String())
	cmd3 := exec.Command("git", "rev-list", fmt.Sprintf("%s..refs/heads/%s", baseId, localBranch))
	cmd3.Dir = gr.GitDirectoryPath
	stdoutBuf.Reset()
	cmd3.Stdout = stdoutBuf
	stderrBuf.Reset()
	cmd3.Stderr = stderrBuf
	err = cmd3.Run()
	if err != nil {
		return nil, fmt.Errorf("Failed to git-rev-list: %s; %s", err, stderrBuf.String())
	}
	ares := strings.TrimSpace(stdoutBuf.String())
	var alist []string
	if ares == "" {
		alist = make([]string, 0)
	} else {
		alist = strings.Split(ares, "\n")
	}
	cmd4 := exec.Command("git", "rev-list", fmt.Sprintf("%s..%s/%s", baseId, remoteName,localBranch))
	cmd4.Dir = gr.GitDirectoryPath
	stdoutBuf.Reset()
	cmd4.Stdout = stdoutBuf
	stderrBuf.Reset()
	cmd4.Stderr = stderrBuf
	err = cmd4.Run()
	if err != nil {
		return nil, fmt.Errorf("Failed to git-rev-list: %s; %s", err, stderrBuf.String())
	}
	bres := strings.TrimSpace(stdoutBuf.String())
	var blist []string
	if bres == "" {
		blist = make([]string, 0)
	} else {
		blist = strings.Split(bres, "\n")
	}
	return &BranchComparisonInfo{
		BaseId: baseId,
		ARevList: alist,
		BRevList: blist,
	}, nil
}

func (gr LocalGitRepository) CheckIfCanFastForward(branch string, remoteName string) (bool, error) {
	hasRemote, err := gr.HasRemote(remoteName)
	if err != nil { return false, err }
	if !hasRemote { return false, nil }
	if !gr.refExists(fmt.Sprintf("refs/remotes/%s/%s", remoteName, branch)) {
		return false, nil
	}
	stderrBuf := new(bytes.Buffer)
	cmd2 := exec.Command("git", "merge-base", fmt.Sprintf("refs/heads/%s", branch), fmt.Sprintf("%s/%s", remoteName, branch))
	cmd2.Dir = gr.GitDirectoryPath
	stdoutBuf := new(bytes.Buffer)
	cmd2.Stdout = stdoutBuf
	stderrBuf.Reset()
	cmd2.Stderr = stderrBuf
	err = cmd2.Run()
	if err != nil {
		return false, fmt.Errorf("Failed to git-merge-base: %s; %s", err, stderrBuf.String())
	}
	base := strings.TrimSpace(stdoutBuf.String())
	localRefPath := path.Join(gr.GitDirectoryPath, "refs", "heads", branch)
	f1, err := os.ReadFile(localRefPath)
	if err != nil {
		return false, fmt.Errorf("Failed to read local branch ref: %s", err)
	}
	return base == strings.TrimSpace(string(f1)), nil
}

// FetchRemoteBranch updates refs/remotes/<remote>/<branch> from the remote.
// Unlike FetchRemote it does not walk every branch of the remote; the update
// hook calls it with the ref a fork push just changed, so the fork page has a
// fresh comparison without fetching itself.
func (gr LocalGitRepository) FetchRemoteBranch(remoteName string, branch string) error {
	refspec := fmt.Sprintf("refs/heads/%s:refs/remotes/%s/%s", branch, remoteName, branch)
	cmd := exec.Command("git", "fetch", "--", remoteName, refspec)
	cmd.Dir = gr.GitDirectoryPath
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Failed to git-fetch %s: %s; %s", branch, err, stderrBuf.String())
	}
	return nil
}

// refExists reports whether a ref can be resolved (including packed refs).
func (gr LocalGitRepository) refExists(ref string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", ref)
	cmd.Dir = gr.GitDirectoryPath
	return cmd.Run() == nil
}

func (gr LocalGitRepository) FetchRemote(remote string) error {
	cmd1 := exec.Command("git", "fetch", remote, "--tags")
	cmd1.Dir = gr.GitDirectoryPath
	stderrBuf := new(bytes.Buffer)
	cmd1.Stderr = stderrBuf
	err := cmd1.Run()
	if err != nil {
		return fmt.Errorf("Failed to git-fetch: %s; %s", err, stderrBuf.String())
	}
	return nil
}

func (gr LocalGitRepository) SyncEmptyRepositoryFromRemote(remote string) error {
	err := gr.FetchRemote(remote)
	if err != nil { return err }
	cmd1 := exec.Command("git", "ls-remote", "--branches", "--tags", remote)
	cmd1.Dir = gr.GitDirectoryPath
	stdout := new(bytes.Buffer)
	cmd1.Stdout = stdout
	err = cmd1.Run()
	if err != nil { return err }
	for v := range strings.SplitSeq(stdout.String(), "\n") {
		p := strings.Split(v, "\t")
		if len(p) < 2 { continue }
		cmd2 := exec.Command("git", "update-ref", p[1], p[0])
		cmd2.Dir = gr.GitDirectoryPath
		err = cmd2.Run()
		if err != nil { return err }
	}
	return nil
}

func (gr LocalGitRepository) UpdateRef(branch string, targetId string) error {
	cmd := exec.Command("git", "update-ref", fmt.Sprintf("refs/heads/%s", branch), targetId)
	cmd.Dir = gr.GitDirectoryPath
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to update-ref: %s; %s", err, stderrBuf.String())
	}
	return nil
}

// two places to check:
//     refs/heads/*,  packed-refs
func (gr LocalGitRepository) GetAllBranchList() (map[string]*Branch, error) {
	var res map[string]*Branch = make(map[string]*Branch)
	rplocal := path.Join(gr.GitDirectoryPath, "refs", "heads")
	ls, err := os.ReadDir(rplocal)
	if err != nil { return nil, err }
	if len(ls) > 0 {
		for _, item := range ls {
			if item.IsDir() { continue }
			name := item.Name()
			f, err := os.Open(path.Join(rplocal, name))
			if err != nil { return nil, err }
			defer f.Close()
			headIdBytes, err := io.ReadAll(f)
			if err != nil { return nil, err }
			headId := strings.TrimSpace(string(headIdBytes))
			res[name] = &Branch{
				Name: name,
				HeadId: headId,
			}
		}
	}
	pi, err := gr.readPackedRefIndex()
	if err != nil { return nil, err }
	if len(pi) > 0 {
		for _, item := range pi {
			// skip tags when counting branches.
			if !strings.HasPrefix(item.Name, "refs/heads/") { continue }
			// update res only if we can't find ready-to-read ref
			name := item.Name[len("refs/heads/"):]
			_, ok := res[name]
			if !ok {
				res[name] = &Branch{
					Name: name,
					HeadId: item.Id,
				}
			}
		}
	}
	return res, nil
}

func (gr *LocalGitRepository) SyncAllBranchList() error {
	br, err := gr.GetAllBranchList()
	if err != nil { return err }
	gr.BranchIndex = br
	return nil
}

func (gr *LocalGitRepository) SyncBranch(branchName string) error {
	rplocal := path.Join(gr.GitDirectoryPath, "refs", "heads")
	headIdBytes, err := os.ReadFile(path.Join(rplocal, branchName))
	if err == nil {
		if gr.BranchIndex == nil {
			gr.BranchIndex = make(map[string]*Branch)
		}
		gr.BranchIndex[branchName] = &Branch{
			Name: branchName,
			HeadId: strings.TrimSpace(string(headIdBytes)),
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) { return err }
	pi, err := gr.readPackedRefIndex()
	if errors.Is(err, os.ErrNotExist) { return nil }
	if err != nil { return err }
	if len(pi) <= 0 { return nil }
	for _, item := range pi {
		if !strings.HasPrefix(item.Name, "refs/heads/") { continue }
		s := strings.TrimPrefix(item.Name, "refs/heads/")
		if s != branchName { continue }
		if gr.BranchIndex == nil {
			gr.BranchIndex = make(map[string]*Branch)
		}
		gr.BranchIndex[branchName] = &Branch{
			Name: branchName,
			HeadId: item.Id,
		}
	}
	return nil
}

// two places to check:
//     refs/tags/*, packed-refs
// afaik fetched remote tags will only appear in packed-refs with
// its id points to the commit object instead of a separate tag
// object like the tags you make locally.
func (gr LocalGitRepository) GetAllTagList() (map[string]*Tag, error) {
	var res map[string]*Tag = make(map[string]*Tag)
	rplocaltags := path.Join(gr.GitDirectoryPath, "refs", "tags")
	ls, err := os.ReadDir(rplocaltags)
	if err != nil { return nil, err }
	if len(ls) > 0 {
		for _, item := range ls {
			if item.IsDir() { continue }
			name := item.Name()
			f, err := os.Open(path.Join(rplocaltags, name))
			if err != nil { return nil, err }
			defer f.Close()
			headIdBytes, err := io.ReadAll(f)
			if err != nil { return nil, err }
			headId := strings.TrimSpace(string(headIdBytes))
			res[name] = &Tag{
				Name: name,
				HeadId: headId,
			}
		}
	}
	pi, err := gr.readPackedRefIndex()
	if err != nil { return nil, err }
	if len(pi) > 0 {
		for _, item := range pi {
			if !strings.HasPrefix(item.Name, "refs/tags/") { continue }
			name := item.Name[len("refs/tags/"):]
			res[name] = &Tag{
				Name: name,
				HeadId: item.Id,
			}
		}
	}
	return res, nil
}

func (gr *LocalGitRepository) SyncAllTagList() error {
	tl, err := gr.GetAllTagList()
	if err != nil { return err }
	gr.TagIndex = tl
	return nil
}

func (gr LocalGitRepository) GetAllObjectId() ([]string, error) {
	var res []string
	op := path.Join(gr.GitDirectoryPath, "objects")
	entryList, err := os.ReadDir(op)
	if err != nil { return nil, err }
	for _, item := range entryList {
		if !item.IsDir() { continue }
		name := item.Name()
		if name == "info" || name == "pack" { continue }
		np := path.Join(op, item.Name())
		subEntryList, err := os.ReadDir(np)
		if err != nil { return nil, err }
		for _, item2 := range subEntryList {
			res = append(res, name + item2.Name())
		}
	}
	return res, nil
}

func (gr LocalGitRepository) GetCommitHistory(cid string) ([]CommitObject, error) {
	return gr.GetCommitHistoryN(cid, 0)
}

func (gr LocalGitRepository) GetCommitHistoryN(cid string, n int) ([]CommitObject, error) {
	var res []CommitObject
	subjId := cid
	limitless := false
	if n <= 0 { limitless = true }
	for (subjId != "" && (limitless || n > 0)) {
		obj, err := gr.ReadObject(subjId)
		if obj.Type() != COMMIT { return nil, err }
		c := obj.(*CommitObject)
		res = append(res, *c)
		subjId = c.ParentId()
		if !limitless { n -= 1 }
	}
	return res, nil
}

func (gr LocalGitRepository) GetBranchCommitHistory(b Branch) ([]CommitObject, error) {
	return gr.GetCommitHistory(b.HeadId)
}

func (gr LocalGitRepository) GetBranchCommitHistoryN(b Branch, n int) ([]CommitObject, error) {
	return gr.GetCommitHistoryN(b.HeadId, n)
}
