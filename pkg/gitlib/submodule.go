package gitlib

import (
	"bytes"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/ini"
)

type SubmoduleConfig struct {
	InRepoPath string
	Url        string
	Branch     string
	Update     string
}

// ParseSubmoduleConfig parses the content of a .gitmodules file. The returned
// map is keyed by the submodule path (the `path` value, not the section name).
func ParseSubmoduleConfig(r io.Reader) (map[string]*SubmoduleConfig, error) {
	i, err := ini.ParseINI(r)
	if err != nil {
		return nil, err
	}
	sms, ok := i.GetSectionList("submodule")
	if !ok {
		return nil, nil
	}
	res := make(map[string]*SubmoduleConfig, 0)
	for _, v := range sms {
		spath := strings.TrimSpace(v.Value["path"])
		if spath == "" {
			spath = strings.TrimSpace(v.SubName)
		}
		if spath == "" {
			continue
		}
		res[spath] = &SubmoduleConfig{
			InRepoPath: spath,
			Url:        strings.TrimSpace(v.Value["url"]),
			Branch:     strings.TrimSpace(v.Value["branch"]),
			Update:     strings.TrimSpace(v.Value["update"]),
		}
	}
	return res, nil
}

// ReadSubmoduleConfigFromTree reads .gitmodules from the root of a commit's
// tree. A bare repository has no .gitmodules on disk; the file is a blob in
// the tree, and it is versioned, so the configuration has to be read from the
// tree that is being viewed.
func (gr LocalGitRepository) ReadSubmoduleConfigFromTree(root *TreeObject) (map[string]*SubmoduleConfig, error) {
	if root == nil {
		return nil, nil
	}
	for i := range root.ObjectList {
		item := &root.ObjectList[i]
		if item.Name != ".gitmodules" {
			continue
		}
		if item.Mode != TREE_NORMAL_FILE && item.Mode != TREE_EXECUTABLE_FILE {
			return nil, errors.New(".gitmodules is not a regular file")
		}
		obj, err := gr.ReadObject(item.Hash)
		if err != nil {
			return nil, err
		}
		blob, ok := obj.(*BlobObject)
		if !ok {
			return nil, errors.New(".gitmodules is not a blob")
		}
		return ParseSubmoduleConfig(bytes.NewReader(blob.Data))
	}
	return nil, nil
}

// SubmoduleURL is the result of resolving a .gitmodules URL.
type SubmoduleURL struct {
	// Raw is the URL exactly as it appears in .gitmodules.
	Raw string
	// FullName is "namespace:name" when the URL resolves to a repository on
	// this Gitus instance. It is empty for an external URL.
	FullName string
	// External is true for an absolute URL, an scp-like URL or any other
	// value that should not be interpreted as a Gitus repository.
	External bool
	// GitusResolved is true when Gitus' own rule (rather than git's URL
	// resolution) produced FullName.
	GitusResolved bool
}

var reGitusFullName = regexp.MustCompile(`^[A-Za-z0-9_-]*:[A-Za-z0-9._-]+$`)
var reScpLike = regexp.MustCompile(`^[^/]+@[^/]+:`)

// ResolveSubmoduleURL resolves a .gitmodules URL for display and linking.
//
// Gitus uses /repo/<namespace>:<name>, so a plain relative URL such as
// "../ghi.git" cannot be resolved by git itself to "abc:ghi": git strips the
// last path component and asks for /repo/ghi.git. Gitus therefore provides
// its own rule for the UI: a relative URL names a repository in the
// superproject's namespace, so the example above resolves to "abc:ghi".
//
// This is a display/link resolution. Git clients still resolve the raw
// .gitmodules URL themselves; docs/submodule.org explains the canonical form
// ("../<namespace>:<name>") that makes recursive clone work.
func ResolveSubmoduleURL(superNamespace string, raw string) SubmoduleURL {
	res := SubmoduleURL{Raw: strings.TrimSpace(raw)}
	if res.Raw == "" {
		return res
	}

	// A Gitus full name, possibly with an empty namespace.
	if reGitusFullName.MatchString(res.Raw) {
		res.FullName = res.Raw
		res.GitusResolved = true
		return res
	}

	// Absolute URL, scheme-relative URL or scp-like URL.
	if strings.Contains(res.Raw, "://") || strings.HasPrefix(res.Raw, "//") || reScpLike.MatchString(res.Raw) {
		res.External = true
		return res
	}

	// Relative URL: strip the query/fragment and use the last path component
	// as the repository name.
	p := res.Raw
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimRight(p, "/")
	base := path.Base(p)
	if base == "" || base == "." || base == ".." {
		return res
	}
	base = strings.TrimSuffix(base, ".git")
	if base == "" {
		return res
	}

	if reGitusFullName.MatchString(base) {
		res.FullName = base
		res.GitusResolved = true
		return res
	}
	if strings.Contains(base, ":") {
		// Not a Gitus full name (the colon is not a namespace separator that
		// we understand) and not an absolute URL. Do not guess.
		res.External = true
		return res
	}
	if superNamespace == "" {
		res.FullName = base
	} else {
		res.FullName = superNamespace + ":" + base
	}
	res.GitusResolved = true
	return res
}

// SuggestedGitURL returns the relative .gitmodules URL that git itself will
// resolve to the same repository on a Gitus instance, or "" when there is no
// such suggestion. For "abc:def" this is "../abc:ghi" for a submodule that
// Gitus resolves to "abc:ghi".
func (s SubmoduleURL) SuggestedGitURL() string {
	if !s.GitusResolved || s.FullName == "" {
		return ""
	}
	return "../" + s.FullName
}
