package gitlib

import (
	"bufio"
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strings"
	"sync"
)

// ResolveLastCommitAtDir returns, for every direct entry of the directory at
// `dir` (a repository-root-relative path, "" for the root), the id of the
// commit that last changed it, looking only at the history reachable from
// `cobj`.
//
// This is meant to replace asking git once per entry: one walk of the history
// resolves the whole listing, and it stops as soon as every entry has been
// accounted for. For a file entry "the last commit that changed it" is the
// newest commit that touched that path; for a directory entry it is the newest
// commit that touched anything underneath it.
//
// The result is memoized: it is a pure function of (repository, commit id,
// directory), and a commit id never changes its history, so a cached entry can
// never go stale. Entries of commits that are no longer browsed are simply
// evicted; nothing has to invalidate them when the repository gets new commits.
//
// Listings are kept in memory (see lastCommitCache below) and, so that a
// restarted server does not have to walk the history again, on disk inside the
// repository (see lastcommit_store.go).
func (gr *LocalGitRepository) ResolveLastCommitAtDir(cobj *CommitObject, dir string) (map[string]string, error) {
	if cobj == nil { return nil, fmt.Errorf("no commit given") }
	dir = normalizeLastCommitDir(dir)
	key := lastCommitCacheKey(gr.GitDirectoryPath, cobj.Id, dir)

	lastCommitCache.mu.Lock()
	if el, ok := lastCommitCache.entries[key]; ok {
		lastCommitCache.lru.MoveToFront(el)
		cached := el.Value.(*lastCommitCacheEntry).value
		lastCommitCache.mu.Unlock()
		return copyLastCommitMap(cached), nil
	}
	if call, ok := lastCommitCache.inflight[key]; ok {
		// Somebody else is already computing this exact listing; wait for it
		// instead of walking the same history again.
		lastCommitCache.mu.Unlock()
		<-call.done
		return copyLastCommitMap(call.res), call.err
	}
	call := &lastCommitCall{done: make(chan struct{})}
	lastCommitCache.inflight[key] = call
	lastCommitCache.mu.Unlock()

	// A previous process may have left the listing on disk; that is much
	// cheaper than walking the history again after a restart.
	diskHit := false
	if cached, ok := gr.loadLastCommitFromDisk(cobj.Id, dir); ok {
		call.res = cached
		diskHit = true
	} else {
		call.res, call.err = gr.resolveLastCommitAtDir(cobj, dir)
	}
	if call.err == nil && !diskHit {
		gr.saveLastCommitToDisk(cobj.Id, dir, call.res)
	}

	lastCommitCache.mu.Lock()
	delete(lastCommitCache.inflight, key)
	if call.err == nil {
		lastCommitCachePutLocked(key, call.res)
	}
	lastCommitCache.mu.Unlock()
	close(call.done)

	return copyLastCommitMap(call.res), call.err
}

// ClearLastCommitCache drops every memoized listing of the process. The on-disk
// listings are not touched; see (*LocalGitRepository).ClearLastCommitStore.
//
// Cached entries are keyed by commit id and cannot go stale, so this is only
// meant for tests, and for operations that rewrite history in place
// (e.g. git replace or grafts).
func ClearLastCommitCache() {
	lastCommitCache.mu.Lock()
	defer lastCommitCache.mu.Unlock()
	lastCommitCache.lru.Init()
	lastCommitCache.entries = make(map[string]*list.Element)
}

// lastCommitCacheCapacity bounds how many directory listings are kept around.
// An entry holds one commit id plus the names of one directory, so a few
// hundred entries stay in the low single-digit megabytes at worst.
const lastCommitCacheCapacity = 512

type lastCommitCacheEntry struct {
	key   string
	value map[string]string
}

type lastCommitCall struct {
	done chan struct{}
	res  map[string]string
	err  error
}

var lastCommitCache = struct {
	mu       sync.Mutex
	lru      *list.List
	entries  map[string]*list.Element
	inflight map[string]*lastCommitCall
}{
	lru:      list.New(),
	entries:  make(map[string]*list.Element),
	inflight: make(map[string]*lastCommitCall),
}

func lastCommitCacheKey(gitDir string, commitId string, dir string) string {
	return gitDir + "\x00" + commitId + "\x00" + dir
}

func lastCommitCachePutLocked(key string, value map[string]string) {
	if value == nil { return }
	if el, ok := lastCommitCache.entries[key]; ok {
		el.Value.(*lastCommitCacheEntry).value = value
		lastCommitCache.lru.MoveToFront(el)
		return
	}
	el := lastCommitCache.lru.PushFront(&lastCommitCacheEntry{key: key, value: value})
	lastCommitCache.entries[key] = el
	for lastCommitCache.lru.Len() > lastCommitCacheCapacity {
		back := lastCommitCache.lru.Back()
		if back == nil { break }
		lastCommitCache.lru.Remove(back)
		delete(lastCommitCache.entries, back.Value.(*lastCommitCacheEntry).key)
	}
}

func copyLastCommitMap(m map[string]string) map[string]string {
	if m == nil { return nil }
	res := make(map[string]string, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}

func normalizeLastCommitDir(dir string) string {
	dir = strings.Trim(dir, "/")
	if len(dir) <= 0 { return "" }
	return path.Clean(dir)
}

// resolveLastCommitAtDir does the actual work; see ResolveLastCommitAtDir.
func (gr *LocalGitRepository) resolveLastCommitAtDir(cobj *CommitObject, dir string) (map[string]string, error) {
	children, err := gr.lastCommitChildren(cobj, dir)
	if err != nil { return nil, err }
	res := make(map[string]string, len(children))
	if len(children) <= 0 { return res, nil }

	// NOTE: `--name-only` never prints a diff for a merge commit unless it is
	// asked to, which is exactly the semantics wanted here: the change came in
	// through one of the merge's parents and that commit is in the same walk.
	//
	// `-z` keeps the output unquoted (a filename may contain anything but NUL),
	// and the \x01 marker in the format keeps commit ids and paths apart
	// without relying on "a 40 character hex string is a commit id".
	args := []string{
		"log", "--no-renames", "--name-only", "-z",
		"--format=%x01%H", cobj.Id,
	}
	if dir != "" {
		// `:(literal)` disables glob matching, otherwise a directory whose
		// name contains `*` or `?` would be taken as a pattern.
		args = append(args, "--", ":(literal)"+dir+"/")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = gr.GitDirectoryPath
	stdout, err := cmd.StdoutPipe()
	if err != nil { return nil, err }
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Start()
	if err != nil { return nil, err }

	reader := bufio.NewReaderSize(stdout, 64*1024)
	expectSeparator := false
	currentCommit := ""
	var readErr error
	for len(res) < len(children) {
		field, err := reader.ReadString(0)
		if len(field) > 0 {
			field = strings.TrimSuffix(field, "\x00")
			if strings.HasPrefix(field, "\x01") {
				currentCommit = strings.TrimSuffix(field[1:], "\n")
				expectSeparator = true
			} else {
				// git prints a newline between the commit header and the list
				// of names, as part of the first name field.
				if expectSeparator {
					field = strings.TrimPrefix(field, "\n")
					expectSeparator = false
				}
				if len(field) > 0 && len(currentCommit) > 0 {
					name := lastCommitDirectChild(field, dir)
					if len(name) > 0 {
						if _, ok := res[name]; !ok {
							res[name] = currentCommit
						}
					}
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) { readErr = err }
			break
		}
	}

	// Everything is resolved, there is no point in letting git keep walking
	// the rest of the history.
	stoppedEarly := len(res) >= len(children)
	if stoppedEarly {
		stdout.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}
	waitErr := cmd.Wait()
	if readErr != nil { return nil, readErr }
	if waitErr != nil && !stoppedEarly {
		return nil, fmt.Errorf("failed to walk the history of %s: %s (%s)",
			cobj.Id, waitErr.Error(), strings.TrimSpace(stderr.String()))
	}
	return res, nil
}

// lastCommitChildren returns the names of the direct entries of `dir` at
// `cobj`, i.e. the entries the caller is going to show.
func (gr *LocalGitRepository) lastCommitChildren(cobj *CommitObject, dir string) ([]string, error) {
	obj, err := gr.ReadObject(cobj.TreeObjId)
	if err != nil { return nil, err }
	root, ok := obj.(*TreeObject)
	if !ok {
		return nil, fmt.Errorf("the tree of commit %s cannot be read as a tree object", cobj.Id)
	}
	tobj := root
	if dir != "" {
		sub, err := gr.ResolveTreePath(root, dir)
		if err != nil { return nil, err }
		tobj, ok = sub.(*TreeObject)
		if !ok {
			return nil, fmt.Errorf("%s in commit %s is not a directory", dir, cobj.Id)
		}
	}
	res := make([]string, 0, len(tobj.ObjectList))
	for _, item := range tobj.ObjectList {
		res = append(res, item.Name)
	}
	return res, nil
}

// lastCommitDirectChild maps a repository-root-relative path that the history
// walk reported to the direct entry of `dir` it belongs to.
func lastCommitDirectChild(p string, dir string) string {
	if dir != "" {
		if !strings.HasPrefix(p, dir+"/") { return "" }
		p = p[len(dir)+1:]
	}
	if len(p) <= 0 { return "" }
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	return p
}
