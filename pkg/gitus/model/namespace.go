package model

import (
	"errors"
	"os"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

type GitusNamespaceStatus int

const (
	NAMESPACE_NORMAL_PUBLIC GitusNamespaceStatus = 1
	NAMESPACE_NORMAL_PRIVATE GitusNamespaceStatus = 2
	NAMESPACE_INTERNAL GitusNamespaceStatus = 3
)

type Namespace struct {
	// name. the id stored in database, the id that appears in urls,
	// also the file name of the parent dir where the repo resides.
	Name string `json:"name"`
	// title. e.g. a namespace with name "mydev-inc" may choose to
	// have the title "MyDev Inc." (which is not valid namespace name.)
	Title string `json:"title"`
	Description string `json:"description"`
	Email string `json:"email"`
	Owner string `json:"owner"`
	RegisterTime int64 `json:"regTime"`
	Status GitusNamespaceStatus `json:"status"`
	ACL *ACL
	RepositoryList map[string]*Repository `json:"repoList"`
	LocalPath string `json:"localPath"`
	// used for reading host mode config only. you should use
	// `.Status` if instance is not in host mode.
	Visibility string `json:"visibility"`
	HostModeACL map[string]*HostModeUserACL `json:"users"`
}

func ValidNamespaceName(s string) bool {
	// NOTE THAT empty string is valid namespace name. empty
	// name namespace is used in the case when the namespace
	// functionality is disabled with config.
	for _, ch := range s {
		if !(
			('a' <= ch && ch <= 'z') ||
				('A' <= ch && ch <= 'Z') ||
				('0' <= ch && ch <= '9') ||
				(ch == '_') ||
				(ch == '-')) {
			return false
		}
	}
	return true
}

func NewNamespace(name string, p string) (*Namespace, error) {
	if !ValidNamespaceName(name) {
		return nil, errors.New("Invalid name for namespace")
	}
	d, err := os.ReadDir(p)
	if os.IsNotExist(err) {
		return nil, errors.New("Namespace path does not exist")
	}
	repoMap := make(map[string]*Repository, 0)
	for _, item := range d {
		repoName := item.Name()
		repoPath := path.Join(p, item.Name())
		if !gitlib.IsValidGitDirectory(repoPath) {
			repoPath = path.Join(p, item.Name(), ".git")
		}
		if !gitlib.IsValidGitDirectory(repoPath) {
			continue
		}
		if strings.HasSuffix(repoName, ".git") {
			repoName = repoName[:len(repoName)-len(".git")]
			if len(repoName) <= 0 {
				continue
			}
		}
		r, err := NewRepository(name, repoName, gitlib.NewLocalGitRepository(repoPath))
		if err != nil { return nil, err }
		repoMap[repoName] = r
	}
	res := &Namespace{
		Name: name,
		Title: name,
		Description: "",
		RepositoryList: repoMap,
		ACL: nil,
		LocalPath: p,
	}
	return res, nil
}

