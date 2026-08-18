package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/ssh"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/shellparse"
	"github.com/GitusCodeForge/Gitus/routes"
)

// handles:
// gitus host-mode keys-update $newrev
// gitus host-mode config-update $newrev
// gitus host-mode gitus-sync $path

func syncHostModeCache(ctx *routes.RouterContext) error {
	gitusSyncDir := path.Join(ctx.Config.GitRoot, "__gitus", "__repo_config", "gitus_sync")
	if ctx.HostModeConfigCache == nil {
		ctx.HostModeConfigCache = make(map[string]*model.HostModeNamespaceConfig, 0)
	}
	lst, err := os.ReadDir(gitusSyncDir)
	if err != nil { return err }
	if !ctx.Config.UseNamespace {
		ctx.HostModeConfigCache[""] = new(model.HostModeNamespaceConfig)
		ctx.HostModeConfigCache[""].RepositoryList = make(map[string]*model.HostModeRepositoryConfig, 0)
		for _, v := range lst {
			if !v.IsDir() { continue }
			if !model.ValidRepositoryName(v.Name()) { continue }
			repoName := v.Name()
			repoDir := path.Join(gitusSyncDir, repoName)
			concreteRepoPath := path.Join(ctx.Config.GitRoot, repoName)
			os.MkdirAll(concreteRepoPath, 0755)
			repoConfPath := path.Join(repoDir, "config.json")
			repoConf, err := model.ReadRepositoryConfigFromFile(repoConfPath)
			if err != nil { return err }
			ctx.HostModeConfigCache[""].RepositoryList[repoName] = repoConf
			_, err = model.CreateLocalRepository(model.REPO_TYPE_GIT, "", repoName, concreteRepoPath)
			if err != nil { return err }
		}
	} else {
		for _, v := range lst {
			if !v.IsDir() { continue }
			if !model.ValidNamespaceName(v.Name()) { continue }
			nsName := v.Name()
			nsDir := path.Join(gitusSyncDir, nsName)
			concreteNsPath := path.Join(ctx.Config.GitRoot, nsName)
			os.MkdirAll(concreteNsPath, 0755)
			nsConfigPath := path.Join(nsDir, "config.json")
			nsConf, err := model.ReadNamespaceConfigFromFile(nsConfigPath)
			if err != nil { return err }
			ctx.HostModeConfigCache[nsName] = nsConf
			nslst, err := os.ReadDir(nsDir)
			if err != nil { return err }
			for _, vv := range nslst {
				if !vv.IsDir() { continue }
				if !model.ValidRepositoryName(vv.Name()) { continue }
				repoName := vv.Name()
				repoConfigDir := path.Join(nsDir, repoName)
				concreteRepoDir := path.Join(ctx.Config.GitRoot, nsName, repoName)
				repoConfPath := path.Join(repoConfigDir, "config.json")
				repoConf, err := model.ReadRepositoryConfigFromFile(repoConfPath)
				if err != nil { return err }
				if nsConf.RepositoryList == nil {
					nsConf.RepositoryList = make(map[string]*model.HostModeRepositoryConfig, 0)
				}
				nsConf.RepositoryList[repoName] = repoConf
				os.MkdirAll(concreteRepoDir, 0755)
				repo, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, nsName, repoName, concreteRepoDir)
				if err != nil { return err }
				gitRepo := repo.(*gitlib.LocalGitRepository)
				for hookName, hookV := range repoConf.Hooks {
					hookFilePath := path.Join(repoConfigDir, hookV)
					hookSource, err := os.ReadFile(hookFilePath)
					if err != nil { return err }
					err = gitRepo.SaveHook(hookName, string(hookSource))
					if err != nil { return err }
				}
				repoDescFilePath := path.Join(concreteRepoDir, "description")
				os.Remove(repoDescFilePath)
				os.WriteFile(repoDescFilePath, []byte(repoConf.Repository.Description), 0644)
			}
		}
	}
	return nil
}

func HandleHostMode(ctx *routes.RouterContext, cmd string, newRev string) {
	switch cmd {
	case "keys-update":
		sshCtx, err := ssh.NewContext(ctx.Config)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to create ssh key context: %s", err))
			return
		}
		ctx.SSHKeyManagingContext = sshCtx
		nsName := ""
		if ctx.Config.UseNamespace { nsName = "__gitus" }
		repoName := "__keys"
		p := path.Join(ctx.Config.GitRoot, nsName, repoName)
		keyGitRepo, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, nsName, repoName, p)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to create access key git repo: %s", err))
			return
		}
		gr := keyGitRepo.(*gitlib.LocalGitRepository)
		gobj, err := gr.ReadObject(newRev)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to access the latest commit of the master branch of key git repo: %s", err))
			return
		}
		treeId := gobj.(*gitlib.CommitObject).TreeObjId
		tobj, err := gr.ReadObject(treeId)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to access the latest commit of the master branch of key git repo: %s", err))
			return
		}
		userList := tobj.(*gitlib.TreeObject).ObjectList
		for _, v := range userList {
			if !model.ValidUserName(v.Name) { continue }
			userdir, err := gr.ReadObject(v.Hash)
			if err != nil { continue }
			userdirobj, ok := userdir.(*gitlib.TreeObject)
			if !ok { continue }
			for _, vv := range userdirobj.ObjectList {
				if vv.Name != "ssh" { continue }
				usersshdir, err := gr.ReadObject(vv.Hash)
				if err != nil { break }
				usersshdirobj, ok := usersshdir.(*gitlib.TreeObject)
				if !ok { break }
				for _, kv := range usersshdirobj.ObjectList {
					obj, err := gr.ReadObject(kv.Hash)
					if err != nil { continue }
					sshCtx.AddAuthorizedKey(v.Name, kv.Name, string(obj.RawData()))
				}
			}
		}
		err = sshCtx.Sync()
		if err != nil {
			printGitError(fmt.Sprintf("Failed to sync ssh keys: %s\n", err));
			return
		}
		
	case "gitus-sync":
		// see docs/host-mode.org
		cmd := exec.Command("git", "pull")
		cmd.Dir = newRev
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_DIR=%s", shellparse.Quote(path.Join(newRev, ".git"))))
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_WORK_DIR=%s", shellparse.Quote(newRev)))
		stdoutBuf := new(bytes.Buffer)
		cmd.Stdout = stdoutBuf
		stderrBuf := new(bytes.Buffer)
		cmd.Stderr = stderrBuf
		err := cmd.Run()
		if err != nil {
			printGitError(fmt.Sprintf("Failed to run git pull at gitus_sync: %s; %s; %s", err, stderrBuf.String(), stdoutBuf.String()))
			return
		}
		err = syncHostModeCache(ctx)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to run git pull at gitus_sync: %s", err))
			return
		}
		
	}
}

