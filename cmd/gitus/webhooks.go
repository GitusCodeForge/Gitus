package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type WebHookEntityInfo struct{
	Name string `json:"name"`
	Email string `json:"email"`
	Username string `json:"username"`
}

type WebHookCommitInfo struct{
	Id string `json:"id"`
	Message string `json:"message"`
	URL string `json:"url"`
	Author WebHookEntityInfo `json:"author"`
	Committer WebHookEntityInfo `json:"committer"`
	Timestamp int64 `json:"timestamp"`
}

type WebHookRepositoryOwnerInfo struct{
	Id int64 `json:"id"`
	Login string `json:"login"`
	FullName string `json:"full_name"`
	Email string `json:"email"`
	UserName string `json:"username"`
}

type WebHookRepositoryInfo struct {
	Id int64 `json:"id"`
	Owner WebHookRepositoryOwnerInfo `json:"owner"`
	Name string `json:"name"`
	Namespace string `json:"namespace"`
	FullName string `json:"full_name"`
	Description string `json:"description"`
	Fork bool `json:"fork"`
	HTMLURL string `json:"html_url"`
	SSHURL string `json:"ssh_url"`
	CloneURL string `json:"clone_url"`
}

type WebHookPayload struct {
	Id string `json:"id"`
	ResultReport string `json:"result_report"`
	ResultReportId string `json:"result_report_id"`
	Reference string `json:"ref"`
	BeforeCommitId string `json:"before"`
	AfterCommitId string `json:"after"`
	CompareURL string `json:"compare_url"`
	Commits []*WebHookCommitInfo `json:"commits"`
	Repository *WebHookRepositoryInfo `json:"repository"`
}

func resolveURL(ctx *routes.RouterContext, repo *model.Repository, cobj *gitlib.CommitObject) string {
	return fmt.Sprintf("%s/repo/%s/commit/%s", ctx.Config.ProperHTTPHostName(), repo.FullName(), cobj.Id)
}

func resolveUsername(ctx *routes.RouterContext, email string) (string, error) {
	s, err := ctx.DatabaseInterface.ResolveEmailToUsername(email)
	if err != nil { return "", err }
	return s, nil
}

func resolveRepoHTMLURL(ctx *routes.RouterContext, repo *model.Repository) string {
	httpHostName := ctx.Config.ProperHTTPHostName()
	rfn := repo.FullName()
	return fmt.Sprintf("%s/repo/%s", httpHostName, rfn)
}

func resolveRepoCloneURL(ctx *routes.RouterContext, repo *model.Repository) string {
	httpHostName := ctx.Config.ProperHTTPHostName()
	rfn := repo.FullName()
	return fmt.Sprintf("%s/repo/%s", httpHostName, rfn)
}

func resolveRepoSSHURL(ctx *routes.RouterContext, repo *model.Repository) string {
	gitSshHostName := ctx.Config.GitSSHHostName()
	sshfn := fmt.Sprintf("%s/%s", repo.Namespace, repo.Name)
	return fmt.Sprintf("%s%s", gitSshHostName, sshfn)
}

// NOTE THAT even if any error happens at this part we still need to
// let the whole program return a success exit code. we can retrigger
// failed cicd later, but whatever pushed to the depot should be accepted
// properly.
func HandleWebHook(ctx *routes.RouterContext, repoFullName string, refFullName string, newrevType string, oldRev string, newRev string) {
	ctx.Config.RecalculateProperPath()
	r := strings.SplitN(repoFullName, ":", 2)
	var ns, name string
	if ctx.Config.UseNamespace {
		ns = r[0]
		name = r[1]
	} else {
		ns = ""
		name = r[0]
	}
	repo, err := ctx.DatabaseInterface.GetRepositoryByName(ns, name)
	if err != nil {
		printGitError(fmt.Sprintf("Failed to get repository: %s", err))
		return
	}
	if !repo.WebHookConfig.Enable { return }
	nonce, err := rand.Int(rand.Reader, big.NewInt(1<<31))
	if err != nil {
		printGitError(fmt.Sprintf("Failed to get repository: %s", err))
		return
	}
	if repo.WebHookConfig.Secret == "" {
		printGitError("Empty secret is not allowed; please check your repository config.")
		return
	}
	reqUuid := uuid.New()
	reportUuid := uuid.New()
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"jti": reportUuid.String(),
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(12 * time.Hour),
		"nonce": nonce.Int64(),
	})
	tokenStr, err := token.SignedString([]byte(repo.WebHookConfig.Secret))
	if err != nil {
		printGitError(fmt.Sprintf("Failed to get repository: %s", err))
		return
	}
	cmd := exec.Command("git", "rev-list", newRev, "^"+oldRev)
	cmd.Dir = repo.Repository.(*gitlib.LocalGitRepository).GitDirectoryPath
	stdoutBuf := new(bytes.Buffer)
	cmd.Stdout = stdoutBuf
	err = cmd.Run()
	if err != nil {
		printGitError(fmt.Sprintf("Failed to get rev list: %s", err))
		return
	}
	commitIdList := strings.Split(stdoutBuf.String(), "\n")
	commits := make([]*WebHookCommitInfo, 0)
	localgr := repo.Repository.(*gitlib.LocalGitRepository)
	for _, k := range commitIdList {
		id := strings.TrimSpace(k)
		if len(id) <= 0 { break }
		gobj, err := localgr.ReadObject(id)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to retrieve rev %s: %s", k, err))
			return
		}
		cobj, ok := gobj.(*gitlib.CommitObject)
		if !ok {
			printGitError(fmt.Sprintf("Failed to retrieve rev %s: %s", k, err))
			return
		}
		authorUsername, _ := resolveUsername(ctx, cobj.AuthorInfo.AuthorEmail)
		committerUsername, _ := resolveUsername(ctx, cobj.CommitterInfo.AuthorEmail)
		commits = append(commits, &WebHookCommitInfo{
			Id: cobj.Id,
			Message: cobj.CommitMessage,
			URL: resolveURL(ctx, repo, cobj),
			Author: WebHookEntityInfo{
				Name: cobj.AuthorInfo.AuthorName,
				Email: cobj.AuthorInfo.AuthorEmail,
				Username: authorUsername,
			},
			Committer: WebHookEntityInfo{
				Name: cobj.CommitterInfo.AuthorName,
				Email: cobj.CommitterInfo.AuthorEmail,
				Username: committerUsername,
			},
			Timestamp: cobj.CommitTime.Unix(),
		})
	}
	owner, err := ctx.DatabaseInterface.GetUserByName(repo.Owner)
	if err != nil {
		printGitError(fmt.Sprintf("Failed to get user: %s", err))
		return
	}
	
	err = ctx.DatabaseInterface.RegisterWebhookRequest(reqUuid.String(), reportUuid.String(), repo.Namespace, repo.Name, newRev)
	if err != nil {
		printGitError(fmt.Sprintf("Failed to register webhook in database: %s", err))
		return
	}
	payload := WebHookPayload{
		Id: reqUuid.String(),
		ResultReport: fmt.Sprintf("%s/%s", ctx.Config.ProperHTTPHostName(), "webhook-result-report"),
		ResultReportId: reportUuid.String(),
		Reference: refFullName,
		BeforeCommitId: oldRev,
		AfterCommitId: newRev,
		Commits: commits,
		Repository: &WebHookRepositoryInfo{
			Id: repo.AbsId,
			Owner: WebHookRepositoryOwnerInfo{
				Id: 0,
				Login: owner.Name,
				FullName: owner.Title,
				Email: owner.Email,
				UserName: owner.Name,
			},
			Description: repo.Description,
			Name: repo.Name,
			Namespace: repo.Namespace,
			FullName: repo.FullName(),
			Fork: repo.ForkOriginName != "" || repo.ForkOriginNamespace != "",
			HTMLURL: resolveRepoHTMLURL(ctx, repo),
			SSHURL: resolveRepoSSHURL(ctx, repo),
			CloneURL: resolveRepoCloneURL(ctx, repo),
		},
	}
	var req *http.Request
	d := &net.Dialer{
		Control: func(network string, address string, c syscall.RawConn) error {
			host, _, _ := net.SplitHostPort(address)
			if auxfuncs.IsPotentiallyLocalAddress(host) {
				if owner.Status != model.SUPER_ADMIN {
					return errors.New("Failed to call webhook due to not enough privilege; please check your configuration.")
				}
			}
			return nil
		},
	}
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: d.DialContext,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			for _, k := range via {
				if auxfuncs.IsPotentiallyLocalAddress(k.Host) {
					if owner.Status != model.SUPER_ADMIN {
						return errors.New("Failed to call webhook due to not enough privilege; please check your configuration")
					}
				}
			}
			if auxfuncs.IsPotentiallyLocalAddress(req.Host) {
				if owner.Status != model.SUPER_ADMIN {
					return errors.New("Failed to call webhook due to not enough privilege; please check your configuration")
				}
			}
			return nil
		},
	}
	switch repo.WebHookConfig.PayloadType {
	case "json":
		payloadJson, err := json.Marshal(payload)
		if err != nil {
			printGitError(fmt.Sprintf("Failed to serialize webhook to json: %s", err))
			return
		}
		rd := bytes.NewReader(payloadJson)
		req, err = http.NewRequest("POST", repo.WebHookConfig.TargetURL, rd)
	default:
		printGitError(fmt.Sprintf("Unsupported webhook payload type: %s", repo.WebHookConfig.PayloadType))
		return
	}
	req.Header.Add("Authentication", fmt.Sprintf("Bearer webhook-jwt-%s", tokenStr))
	req.Header.Add("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		printGitError(fmt.Sprintf("Failed while sending HTTP POST request: %s", err))
		return
	}
	if !strings.HasPrefix(resp.Status, "2") {
		printGitError(fmt.Sprintf("Errorneous HTTP response: %s", resp.Status))
		return
	}
}

