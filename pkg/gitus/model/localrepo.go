package model

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strconv"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

type LocalRepository any

var ErrNotSupported = errors.New("Not supported type of local repository.")

func GetGitusType(x LocalRepository) uint8 {
	_, ok := x.(*gitlib.LocalGitRepository)
	if ok { return REPO_TYPE_GIT }
	return 0
}

func CreateLocalRepository(repoType uint8, namespace string, name string, dirPath string) (LocalRepository, error) {
	switch repoType {
	case REPO_TYPE_GIT:
		return gitlib.NewLocalGitRepository(dirPath), nil
	default:
		return nil, ErrNotSupported
	}
}

func InitLocalRepository(lr LocalRepository) error {
	switch GetGitusType(lr) {
	case REPO_TYPE_GIT:
		cmd := exec.Command("git", "init", "--bare")
		cmd.Dir = GetLocalRepositoryLocalPath(lr)
		return cmd.Run()
	default:
		return ErrNotSupported
	}
}

func CreateLocalForkOf(origin LocalRepository, newNs string, newName string, newP string) (LocalRepository, error) {
	switch GetGitusType(origin) {
	case REPO_TYPE_GIT:
		err := origin.(*gitlib.LocalGitRepository).LocalForkTo(fmt.Sprintf("%s/%s", newNs, newName), newP)
		if err != nil { return nil, err }
		return CreateLocalRepository(REPO_TYPE_GIT, newNs, newName, newP)
	default:
		return nil, ErrNotSupported
	}
}

// InstallManagedHook installs the git hook Gitus uses to send webhooks and to
// warm up its cache. An empty configPath means the caller has no configuration
// file to point the hook at, in which case nothing is installed.
func InstallManagedHook(lr LocalRepository, configPath string, repoFullName string) error {
	if len(configPath) <= 0 { return nil }
	gr, ok := lr.(*gitlib.LocalGitRepository)
	if !ok { return nil }
	return gr.InstallManagedHook(gitlib.ManagedHookParams{
		ConfigPath: configPath,
		RepoFullName: repoFullName,
	})
}

func GetLocalRepositoryLocalPath(r LocalRepository) string {
	switch GetGitusType(r) {
	case REPO_TYPE_GIT:
		return r.(*gitlib.LocalGitRepository).GitDirectoryPath
	default:
		log.Panic(ErrNotSupported)
	}
	return ""
}

func AddFileToRepoString(
	lr LocalRepository,
	branchName string, filePath string,
	authorName string, authorEmail string,
	committerName string, committerEmail string,
	commitMessage string,
	content string,
) (string, error) {
	switch GetGitusType(lr) {
	case REPO_TYPE_GIT:
		glr := lr.(*gitlib.LocalGitRepository)
		return glr.AddFileToRepoString(
			branchName, filePath,
			authorName, authorEmail,
			committerName, committerEmail,
			commitMessage, content,
		)
	default:
		log.Panic(ErrNotSupported)
	}
	return "", nil
}

func AddFileToRepoReader(
	lr LocalRepository,
	branchName string, filePath string,
	authorName string, authorEmail string,
	committerName string, committerEmail string,
	commitMessage string,
	content io.Reader, contentSize int64,
) (string, error) {
	switch GetGitusType(lr) {
	case REPO_TYPE_GIT:
		glr := lr.(*gitlib.LocalGitRepository)
		return glr.AddFileToRepoReader(
			branchName, filePath,
			authorName, authorEmail,
			committerName, committerEmail,
			commitMessage, content, contentSize,
		)
	default:
		log.Panic(ErrNotSupported)
	}
	return "", nil
}

func AddMultipleFileToRepoString(
	lr LocalRepository,
	branchName string,
	authorName string, authorEmail string,
	committerName string, committerEmail string,
	commitMessage string,
	content map[string]string,
) (string, error) {
	switch GetGitusType(lr) {
	case REPO_TYPE_GIT:
		glr := lr.(*gitlib.LocalGitRepository)
		return glr.AddMultipleFileToRepoString(
			branchName,
			authorName, authorEmail,
			committerName, committerEmail,
			commitMessage, content,
		)
	default:
		log.Panic(ErrNotSupported)
	}
	return "", nil
}

func ChangeFileSystemOwnerByName(lr LocalRepository, userName string) error {
	localPath := GetLocalRepositoryLocalPath(lr)
	user, err := user.Lookup(userName)
	if err != nil { return err }
	userUid, _ := strconv.Atoi(user.Uid)
	userGid, _ := strconv.Atoi(user.Gid)
	err = os.Chown(localPath, userUid, userGid)
	if err != nil { return err }
	return nil
}

func ChangeFileSystemOwner(lr LocalRepository, u *user.User) error {
	localPath := GetLocalRepositoryLocalPath(lr)
	userUid, _ := strconv.Atoi(u.Uid)
	userGid, _ := strconv.Atoi(u.Gid)
	err := os.Chown(localPath, userUid, userGid)
	if err != nil { return err }
	return nil
}

