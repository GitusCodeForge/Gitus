package simple_mode

import (
	"path"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

// repo config manager.
// used only in simple mode.

type RepoConfigManager struct {
	rootPath string
	config *gitus.GitusConfig
}

func ToManager(cfg *gitus.GitusConfig) *RepoConfigManager {
	return &RepoConfigManager{
		rootPath: cfg.GitRoot,
		config: cfg,
	}
}

func (rcm *RepoConfigManager) GetNamespace(nsName string) (*model.Namespace, error) {
	var configRelDirPath string
	if rcm.config.UseNamespace {
		configRelDirPath = path.Join("_gitus", "__repo_config", "gitus_sync")
	} else {
		configRelDirPath = path.Join("__repo_config", "gitus_sync")
	}
	nsConfigPath := path.Join(rcm.config.GitRoot, configRelDirPath, nsName, "config.json")
	nsConfig, err := model.ReadNamespaceConfigFromFile(nsConfigPath)
	if err != nil { return nil, err }
	res := new(model.Namespace)
	res.Name = nsName
	res.Title = nsConfig.Namespace.Title
	if res.Title == "" { res.Title = res.Name }
	res.LocalPath = path.Join(rcm.config.GitRoot, nsName)
	if res.Status == 0 {
		switch res.Visibility {
		case "public": res.Status = model.NAMESPACE_NORMAL_PUBLIC
		case "private": res.Status = model.NAMESPACE_NORMAL_PRIVATE
		default: res.Status = model.NAMESPACE_NORMAL_PRIVATE
		}
	}
	return res, nil
}

func (rcm *RepoConfigManager) GetRepository(nsName string, repoName string) (*model.Repository, error) {
	var configRelDirPath string
	if rcm.config.UseNamespace {
		configRelDirPath = path.Join("_gitus", "__repo_config", "gitus_sync")
	} else {
		configRelDirPath = path.Join("__repo_config", "gitus_sync")
	}
	repoConfigPath := path.Join(rcm.config.GitRoot, configRelDirPath, nsName, repoName, "config.json")
	repoConfig, err := model.ReadRepositoryConfigFromFile(repoConfigPath)
	if err != nil { return nil, err }
	res := new(model.Repository)
	res.Type = model.REPO_TYPE_GIT
	res.Name = repoName
	res.Namespace = nsName
	res.Description = repoConfig.Repository.Description
	if res.Status == 0 {
		switch res.Visibility {
		case "public": res.Status = model.REPO_NORMAL_PUBLIC
		case "private": res.Status = model.REPO_NORMAL_PRIVATE
		default: res.Status = model.REPO_NORMAL_PRIVATE
		}
	}
	repoPath := path.Join(rcm.config.GitRoot, nsName, repoName)
	lr, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, nsName, repoName, repoPath)
	if err != nil { return nil, err }
	res.LocalPath = repoPath
	res.Repository = lr
	return res, nil
}


