package model

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

type GitusRepositoryStatus int

const (
	REPO_NORMAL_PUBLIC GitusRepositoryStatus = 1
	REPO_NORMAL_PRIVATE GitusRepositoryStatus = 2
	REPO_ARCHIVED GitusRepositoryStatus = 4
	REPO_INTERNAL GitusRepositoryStatus = 5
	REPO_LIMITED GitusRepositoryStatus = 6
)

const (
	REPO_TYPE_UNKNOWN uint8 = 0
	REPO_TYPE_GIT uint8 = 1
)

func ValidRepositoryName(s string) bool {
	colonPassed := false
	for _, k := range s {
		if !(('0' <= k && k <= '9') || ('A' <= k && k <= 'Z') || ('a' <= k && k <= 'z') || k == '_' || k == '-') {
			if k == ':' {
				if colonPassed { return false }
				colonPassed = true
				continue
			} else {
				return false
			}
		}
	}
	return true
}

func ValidStrictRepositoryName(s string) bool {
	for _, k := range s {
		if !(('0' <= k && k <= '9') || ('A' <= k && k <= 'Z') || ('a' <= k && k <= 'z') || k == '_' || k == '-') {
			return false
		}
	}
	return true
}

type Repository struct {
	AbsId int64 `json:"absid"`
	Type uint8 `json:"type"`
	Namespace string `json:"namespace"`
	Name string `json:"name"`
	Description string `json:"description"`
	AccessControlList *ACL `json:"acl"`
	Owner string `json:"owner"`
	Status GitusRepositoryStatus `json:"status"`
	Repository LocalRepository
	LocalPath string `json:"localPath"`
	ForkOriginNamespace string `json:"forkOriginNamespace"`
	ForkOriginName string `json:"forkOriginName"`
	RepoLabelList []string `json:"labelList"`
	WebHookConfig *WebHookConfig `json:"webHookConfig"`
	// used in host mode only.
	Visibility string `json:"visibility"`
	Users map[string]*HostModeUserACL `json:"users"`
}

func ParseWebHookConfig(s string) (*WebHookConfig, error) {
	var r WebHookConfig
	err := json.Unmarshal([]byte(s), &r)
	if err != nil { return nil, err }
	return &r, nil
}
func (whc *WebHookConfig) String() string {
	r, _ := json.Marshal(whc)
	return string(r)
}

type WebHookConfig struct {
	Enable bool `json:"enable"`
	Secret string `json:"secret"`
	TargetURL string `json:"targetUrl"`
	// the type of the payload. currently only supports "json".
	PayloadType string `json:"payloadType"`
}

func NewRepository(ns string, name string, localgr LocalRepository) (*Repository, error) {
	return &Repository{
		Namespace: ns,
		Name: name,
		Description: "",
		AccessControlList: nil,
		Status: REPO_NORMAL_PUBLIC,
		Repository: localgr,
		LocalPath: GetLocalRepositoryLocalPath(localgr),
	}, nil
}

func (repo *Repository) FullName() string {
	if len(repo.Namespace) > 0 {
		return fmt.Sprintf("%s:%s", repo.Namespace, repo.Name)
	} else {
		return repo.Name
	}
}

func GuessRepositoryType(repoPath string) uint8 {
	if gitlib.IsValidGitDirectory(repoPath) { return REPO_TYPE_GIT }
	if gitlib.IsValidGitDirectory(path.Join(repoPath, ".git")) { return REPO_TYPE_GIT }
	return REPO_TYPE_UNKNOWN
}
