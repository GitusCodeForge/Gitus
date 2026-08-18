package model

import (
	"encoding/json"
	"os"
)

const (
	HOST_MODE_VISIBILITY_PUBLIC = "public"
	HOST_MODE_VISIBILITY_PRIVATE = "private"
	HOST_MODE_PERM_ALLOW = "allow"
	HOST_MODE_PERM_DISALLOW = "disallow"
	HOST_MODE_ACTION_BRANCH_PUSH = "commit"
	HOST_MODE_ACTION_BRANCH_DELETE = "delete"
	HOST_MODE_ACTION_TAG_UNANNONATED = "commit"
	HOST_MODE_ACTION_TAG_DELETE = "delete"
	HOST_MODE_ACTION_TAG_ADD = "tag"
)

type HostModeNamespaceConfig struct {
	Namespace struct {
		Title string `json:"title"`
		Description string `json:"description"`
		Visibility string `json:"visibility"`
	} `json:"namespace"`
	RepositoryList map[string]*HostModeRepositoryConfig
}

type HostModeUserACL struct {
	Default string `json:"default"`
	Push string `json:"push"`
	Pull string `json:"pull"`
	// NOTE(2025.10.31): Patterns are postponed until we come back to the pack format.
	// Pattern map[string][]string `json:"patterns"`
}

type HostModeRepositoryConfig struct {
	Repository struct {
		Description string `json:"description"`
		Visibility string `json:"visibility"`
	} `json:"repo"`
	Hooks map[string]string `json:"hooks"`
	Users map[string]*HostModeUserACL `json:"users"`
}

type HostModeConfigCache map[string]*HostModeNamespaceConfig

func ReadRepositoryConfigFromFile(filePath string) (*HostModeRepositoryConfig, error) {
	s, err := os.ReadFile(filePath)
	if err != nil { return nil, err }
	var res HostModeRepositoryConfig
	err = json.Unmarshal(s, &res)
	if err != nil { return nil, err }
	return &res, nil
}

func ReadNamespaceConfigFromFile(filePath string) (*HostModeNamespaceConfig, error) {
	s, err := os.ReadFile(filePath)
	if err != nil { return nil, err }
	var res HostModeNamespaceConfig
	err = json.Unmarshal(s, &res)
	if err != nil { return nil, err }
	return &res, nil
}

