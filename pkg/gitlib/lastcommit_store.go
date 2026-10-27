package gitlib

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"sort"
	"sync/atomic"
	"time"
)

// The last-commit listing of a directory is cached on disk as well, inside the
// repository it belongs to:
//
//	<git dir>/gitus/lastcommit/<commit>-<directory hash>.json
//
// Keeping it there (instead of a database table or a global cache directory)
// means it is created and removed along with the repository, it is not copied
// by `git clone --bare` (so forks do not inherit entries keyed by a different
// repository), and it is not touched by `git gc` or `git repack`.
//
// Entries are keyed by commit id, which never changes the history behind it, so
// a file can never go stale. A file that cannot be read or does not match the
// commit/directory it was looked up for is simply ignored and recomputed.
const (
	lastCommitStoreVersion = 1
	// gitus/ at the top of a git directory is reserved for things Gitus keeps
	// next to the repository data; git itself ignores it.
	lastCommitStoreDirName = "gitus"
	lastCommitStoreSubDir  = "lastcommit"
	// how many listings are kept per repository before the oldest ones are
	// dropped. A listing of a 100-entry directory is a few kilobytes.
	lastCommitStoreCapacity = 2048
	// pruning walks the store directory, so it only runs every n-th write.
	lastCommitStorePruneInterval = 64
)

type lastCommitStoreFile struct {
	Version   int               `json:"version"`
	Commit    string            `json:"commit"`
	Dir       string            `json:"dir"`
	Timestamp int64             `json:"timestamp"`
	Entries   map[string]string `json:"entries"`
}

var lastCommitStoreWrites atomic.Uint64

// isLastCommitStoreId reports whether s looks like a commit id. The id becomes
// part of a file name, so nothing else is accepted.
func isLastCommitStoreId(s string) bool {
	if len(s) != 40 && len(s) != 64 { return false }
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' { continue }
		if c >= 'a' && c <= 'f' { continue }
		if c >= 'A' && c <= 'F' { continue }
		return false
	}
	return true
}

func (gr *LocalGitRepository) lastCommitStoreDir() string {
	return path.Join(gr.GitDirectoryPath, lastCommitStoreDirName, lastCommitStoreSubDir)
}

func (gr *LocalGitRepository) lastCommitStorePath(commitId string, dir string) string {
	h := sha256.Sum256([]byte(dir))
	return path.Join(gr.lastCommitStoreDir(), commitId+"-"+hex.EncodeToString(h[:8])+".json")
}

// loadLastCommitFromDisk returns the cached listing, if there is a usable one.
func (gr *LocalGitRepository) loadLastCommitFromDisk(commitId string, dir string) (map[string]string, bool) {
	if !isLastCommitStoreId(commitId) { return nil, false }
	b, err := os.ReadFile(gr.lastCommitStorePath(commitId, dir))
	if err != nil { return nil, false }
	var f lastCommitStoreFile
	err = json.Unmarshal(b, &f)
	if err != nil { return nil, false }
	if f.Version != lastCommitStoreVersion { return nil, false }
	if f.Commit != commitId || f.Dir != dir { return nil, false }
	if f.Entries == nil { return nil, false }
	return f.Entries, true
}

// saveLastCommitToDisk writes the listing. Failures are ignored on purpose:
// the store only saves work, it is never needed for correctness.
func (gr *LocalGitRepository) saveLastCommitToDisk(commitId string, dir string, entries map[string]string) {
	if !isLastCommitStoreId(commitId) { return }
	storeDir := gr.lastCommitStoreDir()
	err := os.MkdirAll(storeDir, 0755)
	if err != nil { return }
	b, err := json.Marshal(lastCommitStoreFile{
		Version: lastCommitStoreVersion,
		Commit: commitId,
		Dir: dir,
		Timestamp: time.Now().Unix(),
		Entries: entries,
	})
	if err != nil { return }
	f, err := os.CreateTemp(storeDir, ".tmp-*")
	if err != nil { return }
	tmpName := f.Name()
	defer os.Remove(tmpName)
	_, err = f.Write(b)
	if err != nil { f.Close(); return }
	err = f.Close()
	if err != nil { return }
	// Readers only ever open the final name, so a half-written file is never
	// seen; two writers of the same key produce the same content anyway.
	err = os.Chmod(tmpName, 0644)
	if err != nil { return }
	err = os.Rename(tmpName, gr.lastCommitStorePath(commitId, dir))
	if err != nil { return }
	gr.maybePruneLastCommitStore()
}

func (gr *LocalGitRepository) maybePruneLastCommitStore() {
	if lastCommitStoreWrites.Add(1) % lastCommitStorePruneInterval != 0 { return }
	pruneLastCommitStore(gr.lastCommitStoreDir(), lastCommitStoreCapacity)
}

// ClearLastCommitStore removes the on-disk listings of a repository. Together
// with ClearLastCommitCache this is what is needed after the history behind
// existing commit ids has been rewritten in place (git replace, grafts).
func (gr *LocalGitRepository) ClearLastCommitStore() error {
	ClearLastCommitCache()
	return os.RemoveAll(gr.lastCommitStoreDir())
}

func pruneLastCommitStore(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil { return }
	type storeEntry struct {
		name string
		when time.Time
	}
	files := make([]storeEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() { continue }
		info, err := e.Info()
		if err != nil { continue }
		files = append(files, storeEntry{e.Name(), info.ModTime()})
	}
	if len(files) <= keep { return }
	sort.Slice(files, func(i int, j int) bool { return files[i].when.Before(files[j].when) })
	for _, f := range files[:len(files)-keep] {
		os.Remove(path.Join(dir, f.name))
	}
}
