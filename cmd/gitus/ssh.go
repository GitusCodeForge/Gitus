package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/ssh"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/shellparse"
	"github.com/GitusCodeForge/Gitus/routes"
)

// `gitus ssh` handler.

func printGitError(s string) {
	fmt.Print(gitlib.ToPktLine(fmt.Sprintf("ERR %s\n", s)))
}

func parseTargetRepositoryName(ctx *routes.RouterContext, relPath string) (string, string) {
	if relPath[0] == '~' || relPath[0] == '/' {
		relPath = relPath[1:]
	}
	relPathSegment := strings.SplitN(relPath, "/", 2)
	namespaceName := ""
	repositoryName := ""
	if ctx.Config.UseNamespace {
		if len(relPathSegment) <= 1 {
			relPathSegment = strings.SplitN(relPath, ":", 2)
			if len(relPathSegment) <= 1 {
				printGitError("Invalid repository path specification.")
				os.Exit(1)
			}
			namespaceName = relPathSegment[0]
			repositoryName = relPathSegment[1]
		}
		namespaceName = relPathSegment[0]
		repositoryName = relPathSegment[1]
	} else {
		if len(relPathSegment) > 1 {
			printGitError("Invalid repository path specification.")
			os.Exit(1)
		}
		namespaceName = ""
		repositoryName = relPathSegment[0]
	}
	return namespaceName, repositoryName
}

func isValidGitSSHCommand(s string) bool {
	return (s == "git-upload-pack" || s == "git-receive-pack" || s == "git-upload-archive")
}

func handleSSHSimpleMode(ctx *routes.RouterContext, username string, keyname string) {
	if ctx.SSHKeyManagingContext == nil {
		sshCtx, err := ssh.ToContext(ctx.Config)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to create SSH managing context: %s", err))
			return
		}
		ctx.SSHKeyManagingContext = sshCtx
	}
	origCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	parsedOrigCmd := shellparse.ParseShellCommand(origCmd)
	isPushingToRemote := parsedOrigCmd[0] == "git-receive-pack"
	
	relPath := parsedOrigCmd[len(parsedOrigCmd)-1]
	namespaceName, repositoryName := parseTargetRepositoryName(ctx, relPath)
	var configRelPath string
	if ctx.Config.UseNamespace {
		configRelPath = path.Join("__gitus", "__repo_config", "gitus_sync")
	} else {
		configRelPath = path.Join("__repo_config", "gitus_sync")
	}
	configPath := path.Join(ctx.Config.GitRoot, configRelPath, namespaceName, repositoryName, "config.json")
	config, err := model.ReadRepositoryConfigFromFile(configPath)
	if err != nil {
		printGitError(fmt.Sprintf("Failed to read repository config: %s", err))
		return
	}
	userAcl, ok := config.Users[username]
	if !ok {
		printGitError("Not enough permission")
		return
	}
	var allowVerdict string
	if isPushingToRemote {
		allowVerdict = userAcl.Push
	} else {
		allowVerdict = userAcl.Pull
	}
	if allowVerdict == "" { allowVerdict = userAcl.Default }

	switch allowVerdict {
	case "allow":
		// see also:
		//     https://git-scm.com/docs/git-receive-pack
		//     https://git-scm.com/docs/git-upload-pack
		//     https://git-scm.com/docs/git-upload-archive
		// all commands have the git dir path at the end of the call, so we resolve it
		// with ctx.Config.
		realGitPath := path.Join(ctx.Config.GitRoot, namespaceName, repositoryName)
		parsedOrigCmd[len(parsedOrigCmd)-1] = realGitPath
		cmdobj := exec.Command(parsedOrigCmd[0], parsedOrigCmd[1:]...)

		cmdobj.Stdout = os.Stdout
		cmdobj.Stdin = os.Stdin
		cmdobj.Stderr = os.Stderr
		err = cmdobj.Run()
		if err != nil {
			printGitError(err.Error())
		}
		os.Exit(0)
	case "disallow": fallthrough
	default:
		printGitError("Not enough permission")
		return
	}
	
}

func HandleSSHLogin(ctx *routes.RouterContext, username string, keyname string) {
	if ctx.Config.OperationMode == gitus.OP_MODE_SIMPLE {
		handleSSHSimpleMode(ctx, username, keyname)
		return
	}
	if ctx.Config.IsInPlainMode() {
		printGitError("This instance of Gitus is in Plain Mode which does not allow Git over SSH.")
		os.Exit(1)
	}
	if ctx.Config.GlobalVisibility != gitus.GLOBAL_VISIBILITY_PUBLIC &&
		ctx.Config.GlobalVisibility != gitus.GLOBAL_VISIBILITY_PRIVATE {
		printGitError("This instance of Gitus is currently unavailable.")
		os.Exit(1)
	}
	m, err := ctx.DatabaseInterface.GetAuthKeyByName(username, keyname)
	if err != nil {
		printGitError(err.Error())
		os.Exit(1)
	}
	authorizedKey := ctx.SSHKeyManagingContext.GetAuthorizedKey(username, keyname)
	if authorizedKey != m.KeyText {
		printGitError(fmt.Sprintf("Integrity check failed:\n auth: %s\nkt: %s", authorizedKey, m.KeyText))
		os.Exit(1)
	}
	origCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	// one might be tempted to think that one can just pass SSH_ORIGINAL_COMMAND
	// to exec.Command, but things don't work that way...
	parsedOrigCmd := shellparse.ParseShellCommand(origCmd)
	if len(parsedOrigCmd) <= 0 {
		printGitError("Invalid SSH command")
		os.Exit(1)
	}
	// need to have a guard here or else normal users might get to
	// execute commands as the git user due to incorrect acl config.
	if !isValidGitSSHCommand(parsedOrigCmd[0]) {
		printGitError("Invalid SSH command")
		os.Exit(1)
	}
	
	isPushingToRemote := parsedOrigCmd[0] == "git-receive-pack"
	relPath := parsedOrigCmd[len(parsedOrigCmd)-1]
	namespaceName, repositoryName := parseTargetRepositoryName(ctx, relPath)

	// check acl.
	r, err := ctx.DatabaseInterface.GetRepositoryByName(namespaceName, repositoryName)
	if err != nil {
		printGitError(fmt.Sprintf("Failed while reading ACL: %s.", err.Error()))
		os.Exit(1)
	}
	if r.Status == model.REPO_ARCHIVED && isPushingToRemote {
		printGitError(fmt.Sprintf("The repository %s/%s is ARCHIVED; no push to remote is allowed. ", namespaceName, repositoryName))
		os.Exit(1)
	}
	ns, err := ctx.DatabaseInterface.GetNamespaceByName(namespaceName)
	if err != nil {
		printGitError(fmt.Sprintf("Failed while reading namespace: %s.", err.Error()))
		os.Exit(1)
	}
	if r.Owner != username && ns.Owner != username {
		aclt, ok := r.AccessControlList.ACL[username]
		if !ok {
			aclt, ok = ns.ACL.ACL[username]
			if !ok {
				printGitError("Not enough permission.")
				os.Exit(1)
			}
		}
		if !aclt.PushToRepository && isPushingToRemote {
			printGitError("Not enough permission.")
			os.Exit(1)
		}
	}

	// see also:
	//     https://git-scm.com/docs/git-receive-pack
	//     https://git-scm.com/docs/git-upload-pack
	//     https://git-scm.com/docs/git-upload-archive
	// all commands have the git dir path at the end of the call, so we resolve it
	// with ctx.Config.
	realGitPath := path.Join(ctx.Config.GitRoot, r.Namespace, r.Name)
	parsedOrigCmd[len(parsedOrigCmd)-1] = realGitPath
	cmdobj := exec.Command(parsedOrigCmd[0], parsedOrigCmd[1:]...)

	cmdobj.Stdout = os.Stdout
	cmdobj.Stdin = os.Stdin
	cmdobj.Stderr = os.Stderr
	err = cmdobj.Run()
	if err != nil {
		printGitError(err.Error())
	}
	os.Exit(0)
}
