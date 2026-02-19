package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	dbinit "github.com/GitusCodeForge/Gitus/pkg/gitus/db/init"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	rsinit "github.com/GitusCodeForge/Gitus/pkg/gitus/receipt/init"
	ssinit "github.com/GitusCodeForge/Gitus/pkg/gitus/session/init"
	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/shellparse"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)

type WebInstallerRoutingContext struct {
	Template *template.Template
	// yes, we do share the same object between multiple goroutie,
	// but i don't think this would be a problem for a simple web
	// installer.
	// step 1 - plain mode or non-plain mode?
	//          use namespace or not?
	//          plain mode - goto step [6]
	// step 2 - database config
	// step 3 - session config
	// step 4 - mailer config
	// step 5 - receipt system config
	// step 6 - git-related config
	// step 7 - ignored namespaces & repositories
	// step 8 - web front setup:
	//          depot name
	//          rate limiter
	//          front page config
	//          (static assets dir default to be $HOME/gitus-static/)
	//          bind address & port
	//          http host name
	//          ssh host name (disabled if plain mode)
	//          allow registration
	//          email confirmation required
	//          manual approval
	// step 9 - confirm code manager setup
	// step 10 - root ssh key setup
	// plain mode: 1-6-7-8
	// simple mode: 1-6-8-10
	// normal mode: 1-2-3-4-5-6-8-9
	Step int
	Config *gitus.GitusConfig
	ConfirmStageReached bool
	ResultingFilePath string
	GitUserHome string
	RootSSHKey string
}

func logTemplateError(e error) {
	if e != nil { log.Print(e) }
}

func (ctx *WebInstallerRoutingContext) loadTemplate(name string) *template.Template {
	return ctx.Template.Lookup(name)
}

func withLog(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf(" %s %s\n", r.Method, r.URL.Path)
		f(w, r)
	}
}

func foundAt(w http.ResponseWriter, p string) {
	w.Header().Add("Content-Length", "0")
	w.Header().Add("Location", p)
	w.WriteHeader(302)
}

func (ctx *WebInstallerRoutingContext) reportRedirect(target string, timeout int, title string, message string, w http.ResponseWriter) {
	logTemplateError(ctx.loadTemplate("webinstaller/_redirect").Execute(w, templates.WebInstRedirectWithMessageModel{
		Timeout: timeout,
		RedirectUrl: target,
		MessageTitle: title,
		MessageText: message,
	}))
}

func bindAllWebInstallerRoutes(ctx *WebInstallerRoutingContext) {
	http.HandleFunc("GET /", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/start").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	
	http.HandleFunc("GET /step1", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step1").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step1", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step1", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		om := strings.TrimSpace(r.Form.Get("operation-mode"))
		if om == "" {
			ctx.reportRedirect("/step1", 5, "Invalid Request", "Operation mode must be one of \"plain\", \"simple\" and \"normal\"", w)
			return
		}
		ctx.Config.OperationMode = om
		ctx.Config.UseNamespace = len(r.Form.Get("enable-namespace")) > 0
		switch ctx.Config.OperationMode {
		case gitus.OP_MODE_NORMAL:
			foundAt(w, "/step2")
		case gitus.OP_MODE_PLAIN:
			foundAt(w, "/step6")
		case gitus.OP_MODE_SIMPLE:
			foundAt(w, "/step6")
		}
	}))
	
	http.HandleFunc("GET /step2", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step2").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step2", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step2", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.Database = gitus.GitusDatabaseConfig{
			Type: strings.TrimSpace(r.Form.Get("database-type")),
			Path: strings.TrimSpace(r.Form.Get("database-path")),
			URL: strings.TrimSpace(r.Form.Get("database-url")),
			UserName: strings.TrimSpace(r.Form.Get("database-username")),
			DatabaseName: strings.TrimSpace(r.Form.Get("database-database-name")),
			TablePrefix: strings.TrimSpace(r.Form.Get("database-table-prefix")),
			Password: strings.TrimSpace(r.Form.Get("database-password")),
		}

 		foundAt(w, "/step3")
	}))
	
	http.HandleFunc("GET /step3", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step3").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step3", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step3", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		i, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("session-database-number")), 10, 32)
		if err != nil {
			ctx.reportRedirect("/step3", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.Session = gitus.GitusSessionConfig{
			Type: strings.TrimSpace(r.Form.Get("session-type")),
			Path: strings.TrimSpace(r.Form.Get("session-path")),
			TablePrefix: strings.TrimSpace(r.Form.Get("session-table-prefix")),
			Host: strings.TrimSpace(r.Form.Get("session-host")),
			UserName: strings.TrimSpace(r.Form.Get("session-username")),
			Password: strings.TrimSpace(r.Form.Get("session-password")),
			DatabaseNumber: int(i),
		}
		foundAt(w, "/step4")
	}))

	
	http.HandleFunc("GET /step4", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step4").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step4", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step4", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		i, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("mailer-smtp-port")), 10, 32)
		if err != nil {
			ctx.reportRedirect("/step4", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.Mailer = gitus.GitusMailerConfig{
			Type: strings.TrimSpace(r.Form.Get("mailer-type")),
			SMTPServer: strings.TrimSpace(r.Form.Get("mailer-smtp-server")),
			SMTPPort: int(i),
			SMTPAuth: strings.TrimSpace(r.Form.Get("mailer-smtp-auth")),
			User: strings.TrimSpace(r.Form.Get("mailer-user")),
			Password: strings.TrimSpace(r.Form.Get("mailer-password")),
		}
		foundAt(w, "/step5")
	}))
	
	http.HandleFunc("GET /step5", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step5").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step5", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step5", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.ReceiptSystem = gitus.GitusReceiptSystemConfig{
			Type: strings.TrimSpace(r.Form.Get("receipt-system-type")),
			Path: strings.TrimSpace(r.Form.Get("receipt-system-path")),
			URL: strings.TrimSpace(r.Form.Get("receipt-system-url")),
			UserName: strings.TrimSpace(r.Form.Get("receipt-system-username")),
			DatabaseName: strings.TrimSpace(r.Form.Get("receipt-system-database-name")),
			Password: strings.TrimSpace(r.Form.Get("receipt-system-password")),
			TablePrefix: strings.TrimSpace(r.Form.Get("receipt-system-table-prefix")),
		}
		foundAt(w, "/step6")
	}))
	
	http.HandleFunc("GET /step6", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step6").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step6", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step6", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.GitRoot = strings.TrimSpace(r.Form.Get("git-root"))
		ctx.Config.GitUser = strings.TrimSpace(r.Form.Get("git-user"))
		ctx.Config.SnippetRoot = strings.TrimSpace(r.Form.Get("snippet-root"))
		ctx.Config.GitConfig.HTTPCloneProtocol.V1Dumb = len(strings.TrimSpace(r.Form.Get("git-http-clone-enable-v1-dumb"))) > 0
		ctx.Config.GitConfig.HTTPCloneProtocol.V2 = len(strings.TrimSpace(r.Form.Get("git-http-clone-enable-v2"))) > 0
		next := ""
		if ctx.Config.IsInPlainMode() {
			next = "/step7"
		} else {
			next = "/step8"
		}
		ctx.Config.NoInteractiveShellMessage = strings.TrimSpace(r.Form.Get("no-interactive-shell-message"))
		err = templates.UnpackStaticFileTo(ctx.Config.StaticAssetDirectory)
		if err != nil {
			ctx.reportRedirect(next, 0, "Failed", fmt.Sprintf("Static file unpack is unsuccessful due to reason: %s. You can still move forward but would have to unpack static file yourself.", err.Error()), w)
			return
		}
		foundAt(w, next)
	}))
	
	http.HandleFunc("GET /step7", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step7").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step7", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step1", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.IgnoreNamespace = make([]string, 0)
		for k := range strings.SplitSeq(r.Form.Get("ignore-namespace"), ",") {
			ctx.Config.IgnoreNamespace = append(ctx.Config.IgnoreNamespace, k)
		}
		ctx.Config.IgnoreRepository = make([]string, 0)
		for k := range strings.SplitSeq(r.Form.Get("ignore-repository"), ",") {
			ctx.Config.IgnoreRepository = append(ctx.Config.IgnoreRepository, k)
		}
		foundAt(w, "/step8")
	}))
	
	http.HandleFunc("GET /step8", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step8").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step8", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step8", 0, "Invalid Request", "The request is of an invalid form. Please try again. " + err.Error(), w)
			return
		}
		ctx.Config.DepotName = strings.TrimSpace(r.Form.Get("depot-name"))
		ctx.Config.BindAddress = strings.TrimSpace(r.Form.Get("bind-address"))
		i, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("bind-port")), 10, 32)
		if err != nil {
			ctx.reportRedirect("/step8", 0, "Invalid Request", "The request is of an invalid form. Please try again." + err.Error(), w)
			return
		}
		ctx.Config.BindPort = int(i)
		ctx.Config.HttpHostName = strings.TrimSpace(r.Form.Get("http-host-name"))
		ctx.Config.SshHostName = strings.TrimSpace(r.Form.Get("ssh-host-name"))
		ctx.Config.FrontPage.Type = strings.TrimSpace(r.Form.Get("front-page-type"))
		ctx.Config.FrontPage.Namespace = strings.TrimSpace(r.Form.Get("front-page-namespace"))
		ctx.Config.FrontPage.Repository = strings.TrimSpace(r.Form.Get("front-page-repository"))
		ctx.Config.FrontPage.FileContent = r.Form.Get("front-page-file-content")
		ctx.Config.Theme.ForegroundColor = "#000000"
		ctx.Config.Theme.BackgroundColor = "#ffffff"
		// NOTE: these options are not used in plain mode and simple mode.
		if ctx.Config.OperationMode == gitus.OP_MODE_NORMAL {
			ctx.Config.AllowRegistration = len(strings.TrimSpace(r.Form.Get("allow-registration"))) > 0
			ctx.Config.EmailConfirmationRequired = len(strings.TrimSpace(r.Form.Get("email-confirmation-required"))) > 0
			ctx.Config.ManualApproval = len(strings.TrimSpace(r.Form.Get("manual-approval"))) > 0
			us, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("default-new-user-status")), 10, 32)
			if err != nil {
				ctx.reportRedirect("/step8", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
				return
			}
			ctx.Config.DefaultNewUserStatus = model.GitusUserStatus(us)
			ctx.Config.DefaultNewUserNamespace = strings.TrimSpace(r.Form.Get("default-new-user-namespace"))
		}
		maxr, err := strconv.ParseFloat(strings.TrimSpace(r.Form.Get("max-request-in-second")), 64)
		if err != nil {
			ctx.reportRedirect("/step8", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.MaxRequestInSecond = maxr
		switch ctx.Config.OperationMode {
		case gitus.OP_MODE_PLAIN:
			foundAt(w, "/confirm")
		case gitus.OP_MODE_SIMPLE:
			foundAt(w, "/step10")
		case gitus.OP_MODE_NORMAL:
			foundAt(w, "/step9")
		}
	}))

	http.HandleFunc("GET /step9", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step9").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
		}))
	}))

	http.HandleFunc("POST /step9", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step9", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		i, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("default-timeout-minute")), 10, 32)
		if err != nil {
			ctx.reportRedirect("/step9", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.ConfirmCodeManager.Type = strings.TrimSpace(r.Form.Get("type"))
		ctx.Config.ConfirmCodeManager.DefaultTimeoutMinute = int(i)
		foundAt(w, "/confirm")
	}))
	
	http.HandleFunc("GET /step10", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step10").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
		}))
	}))
	
	http.HandleFunc("POST /step10", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step10", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		rootssh := strings.TrimSpace(r.Form.Get("root-ssh"))
		ctx.RootSSHKey = rootssh
		foundAt(w, "/confirm")
	}))
	
	http.HandleFunc("GET /confirm", withLog(func(w http.ResponseWriter, r *http.Request) {
		ctx.ConfirmStageReached = true
		logTemplateError(ctx.loadTemplate("webinstaller/confirm").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
			RootSSHKey: ctx.RootSSHKey,
		}))
	}))

	http.HandleFunc("GET /install", withLog(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>Gitus Web Installer</title>`)
		ctx.loadTemplate("webinstaller/_style").Execute(w, nil)
		fmt.Fprint(w, `
  </head>
  <body>
    <header>
	  <h1><a href="/">Gitus Web Installer</a></h1>
	  <ul>
        <li><a href="/step1">Step 1: Operation Mode &amp; Enabling Namespace</a></li>
        <li><a href="/step2">Step 2: Database Config</a></li>
        <li><a href="/step3">Step 3: Session Config</a></li>
        <li><a href="/step4">Step 4: Mailer Config</a></li>
        <li><a href="/step5">Step 5: Receipt System Config</a></li>
        <li><a href="/step6">Step 6: Git Root &amp; Git User</a></li>
        <li><a href="/step7">Step 7: Ignored Namespace/Repositories</a></li>
        <li><a href="/step8">Step 8: Misc. Setup</a></li>
        <li><a href="/step9">Step 9: Confirm Code Manager Setup</li>
        <li><a href="/step9">Step 10: Root SSH Key Setup</li>
        <li><a href="/confirm">Confirm</a></li>
      </ul>
	</header>

	<hr />
`)
		if len(strings.TrimSpace(ctx.Config.GitUser)) <= 0 {
			fmt.Fprint(w, "<p>Git user empty. Please fix this...</p>")
			goto leave
		}
		if !func()bool{
			_, err := user.Lookup(ctx.Config.GitUser)
			if err == nil { return true }
			fmt.Fprint(w, "<p>Creating Git user...</p>")
			gitShellPath, err := whereIs("git-shell")
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to search for git-shell: %s</p>", err.Error())
				return false
			}
			if len(gitShellPath) <= 0 {
				fmt.Fprint(w, "<p>Failed to search for git-shell: git-shell path empty.</p>")
				return false
			}
			homePath := fmt.Sprintf("/home/%s", ctx.Config.GitUser)
			ctx.Config.StaticAssetDirectory = path.Join(homePath, "gitus-static-assets")
			err = os.MkdirAll(homePath, os.ModeDir|0755)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to create home directory for user %s: %s</p>", ctx.Config.GitUser, homePath)
				return false
			}
			useraddPath, err := whereIs("useradd")
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to find command \"useradd\": %s</p>", err.Error())
				return false
			}
			if len(useraddPath) <= 0 {
				fmt.Fprint(w, "<p>Failed to find command \"useradd\": useradd path empty")
				return false
			}
			cmd := exec.Command(useraddPath, "-d", homePath, "-m", "-s", gitShellPath, ctx.Config.GitUser)
			err = cmd.Run()
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to run useradd: %s</p>", err.Error())
				return false
			}
			return true
		}() { goto leave }

		if !func()bool{
			gitUser, err := user.Lookup(ctx.Config.GitUser)
			if err != nil {
				fmt.Fprintf(w, "<p>Somehow failed to retrieve user after registering: %s\n", err.Error())
				return false
			}
			homePath := gitUser.HomeDir
			uid, _ := strconv.Atoi(gitUser.Uid)
			gid, _ := strconv.Atoi(gitUser.Gid)
			fmt.Fprint(w,"<p>Chown-ing git user home directory...</p>")
			err = os.Chown(homePath, int(uid), int(gid))
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to chown the git user home directory: %s</p>", err.Error())
				return false
			}
			fmt.Fprint(w, "<p>Creating git-shell-commands directory...</p>")
			gitShellCommandPath := path.Join(homePath, "git-shell-commands")
			err = createOtherOwnedDirectory(gitShellCommandPath, gitUser.Uid, gitUser.Gid)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to chown the git shell command directory: %s</p>", err.Error())
				return false
			}
			fmt.Fprint(w, "<p>Creating .ssh directory...</p>")
			sshPath := path.Join(homePath, ".ssh")
			err = createOtherOwnedDirectory(sshPath, gitUser.Uid, gitUser.Gid)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to create the .ssh folder: %s</p>", err.Error())
				return false
			}
			fmt.Fprint(w, "<p>Creating authorized_keys file...</p>")
			authorizedKeysPath := path.Join(homePath, ".ssh", "authorized_keys")
			err = createOtherOwnedFile(authorizedKeysPath, gitUser.Uid, gitUser.Gid)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to create the authorized_keys file: %s</p>", err.Error())
				return false
			}
			fmt.Fprint(w, "<p>Copying gitus executable...</p>")
			s, err := os.Executable()
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to copy Gitus executable: %s</p>", err.Error())
				return false
			}
			gitusPath := path.Join(homePath, "git-shell-commands", "gitus")
			if gitusPath == s {
				fmt.Fprint(w, "<p>Seems like executable already exists. Not copying...</p>\n")
			} else {
				f, err := os.Open(s)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to copy Gitus executable: %s</p>", err.Error())
					return false
				}
				defer f.Close()
				fout, err := os.OpenFile(gitusPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0754)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to copy Gitus executable: %s\n</p>", err.Error())
					return false
				}
				defer fout.Close()
				_, err = io.Copy(fout, f)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to copy Gitus executable: %s\n</p>", err.Error())
					return false
				}
				err = os.Chown(gitusPath, uid, gid)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to copy Gitus executable: %s\n</p>", err.Error())
					return false
				}
			}
			
			err = os.MkdirAll(ctx.Config.GitRoot, os.ModeDir|0755)
			if errors.Is(err, os.ErrExist) {
				err = os.Chown(ctx.Config.GitRoot, uid, gid)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to chown git root: %s\n</p>", err.Error())
					return false
				}
			}
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to chown git root: %s\n</p>", err.Error())
				return false
			}
			ctx.Config.FilePath = path.Join(homePath, fmt.Sprintf("gitus-config-%d.json", time.Now().Unix()))
			fmt.Fprint(w, "<p>Git user setup done.</p>")
			ctx.Config.RecalculateProperPath()
			err = ctx.Config.Sync()
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to save config file: %s\n. You might need to do this again or even manually.</p>", err.Error())
				return false
			}
			err = auxfuncs.ChangeLocationOwnerByName(ctx.Config.FilePath, ctx.Config.GitUser)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to change config file owner: %s. You should do this after the installation process has completed</p>", err)
			}
			
			noInteractiveLoginPath := path.Join(homePath, "git-shell-commands", "no-interactive-login")
			f, err := os.OpenFile(noInteractiveLoginPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0754)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to write <code>no-interactive-login</code>: %s; interactive shell would still be available. If this is undesirable, you'll have to add it yourself.</p>", err.Error())
			} else {
				defer f.Close()
				fmt.Fprintf(f, `#!/bin/sh

%s -config "%s" no-login
`, path.Join(homePath, "git-shell-commands", "gitus"),
					shellparse.Quote(ctx.Config.FilePath),
				)
				os.Chown(noInteractiveLoginPath, uid, gid)
				fmt.Fprint(w, "<p><code>no-interactive-login</code> file has been written successfully.</p>")
			}
			return true
		}() { goto leave }

		if !func()bool{
			if ctx.Config.OperationMode != gitus.OP_MODE_NORMAL {
				return true
			}
			fmt.Fprint(w, "<p>Initializing database...</p>")
			dbif, err := dbinit.InitializeDatabase(ctx.Config)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to initialize database: %s</p>", err.Error())
				return false
			}
			defer dbif.Dispose()
			chkres, err := dbif.IsDatabaseUsable()
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to initialize database: %s</p>", err.Error())
				return false
			}
			if !chkres {
				err = dbif.InstallTables()
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to initialize database: %s</p>", err.Error())
					return false
				}
			}
			
			fmt.Fprint(w, "<p>Initialization done.</p>")
			return true
		}() { goto leave }
		
		if !func()bool{
			if ctx.Config.OperationMode != gitus.OP_MODE_NORMAL {
				return true
			}
			fmt.Fprint(w, "<p>Initializing session store...</p>")
			ssif, err := ssinit.InitializeDatabase(ctx.Config)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to initialize session store: %s</p>", err.Error())
				return false
			}
			defer ssif.Dispose()
			chkres, err := ssif.IsSessionStoreUsable()
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to initialize session store: %s</p>", err.Error())
				return false
			}
			if !chkres {
				err = ssif.Install()
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to initialize session store: %s</p>", err.Error())
					return false
				}
			}
			fmt.Fprint(w, "<p>Initialization done.</p>")
			return true
		}() { goto leave }
		
		if !func()bool{
			if ctx.Config.OperationMode != gitus.OP_MODE_NORMAL {
				return true
			}
			w.Write([]byte("<p>Initializing receipt system...</p>"))
			rsif, err := rsinit.InitializeReceiptSystem(ctx.Config)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to initialize receipt system: %s</p>", err.Error())
				return false
			}
			defer rsif.Dispose()
			chkres, err := rsif.IsReceiptSystemUsable()
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to initialize receipt system: %s</p>", err.Error())
				return false
			}
			if !chkres {
				err = rsif.Install()
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to initialize receipt system: %s</p>", err.Error())
					return false
				}
			}
			fmt.Fprint(w, "<p>Initialization done.</p>")
			return true
		}() { goto leave }
		
		if !func()bool{
			if ctx.Config.OperationMode != gitus.OP_MODE_NORMAL {
				return true
			}
			fmt.Fprint(w, "<p>Setting up admin user.</p>")
			dbif, err := dbinit.InitializeDatabase(ctx.Config)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to open database while setting up admin user: %s</p>", err.Error())
				return false
			}
			defer dbif.Dispose()
			adminExists := false
			_, err = dbif.GetUserByName("admin")
			if err == db.ErrEntityNotFound {
				adminExists = false
			} else if err != nil {
				fmt.Fprintf(w, "<p>Failed to check database while setting up admin user: %s</p>", err.Error())
				return false
			} else {
				adminExists = true
			}
			if adminExists {
				err = dbif.HardDeleteUserByName("admin")
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to remove original admin user while setting up new admin user: %s</p>", err.Error())
					return false
				}
			}
			userPassword := mkpass()
			r, err := bcrypt.GenerateFromPassword([]byte(userPassword), bcrypt.DefaultCost)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to generate password: %s</p>", err.Error())
				return false
			}
			_, err = dbif.RegisterUser("admin", "", string(r), model.SUPER_ADMIN)
			if err != nil {
				fmt.Fprintf(w, "<p>Failed to register user: %s</p>", err.Error())
				return false
			}
			if len(ctx.Config.DefaultNewUserNamespace) > 0 {
				n := ctx.Config.DefaultNewUserNamespace
				_, err := dbif.RegisterNamespace(n, "admin")
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to create default namespace: %s</p>", err)
					return false
				}
			}
			fmt.Fprintf(w, "<p>Admin user set up properly.</p><pre>Username: admin\nPassword: %s</pre><p>Please copy the password above because we don't store the plaintext; but, in the case you forgot, you can always run the following command to reset the admin user's password:</p><pre>gitus -config %s reset-admin</pre>", userPassword, ctx.Config.FilePath)
			return true
		}() { goto leave }

		if !func()bool{
			gitUser, _ := user.Lookup(ctx.Config.GitUser)
			var uid int
			var gid int
			if gitUser != nil {
				uid, _ = strconv.Atoi(gitUser.Uid)
				gid, _ = strconv.Atoi(gitUser.Gid)
			}
			if ctx.Config.Database.Type == "sqlite" {
				if gitUser == nil {
					fmt.Fprint(w, "<p class=\"warning\">Failed to fild Git user's uid & gid when chowning sqlite database. You need to perform this action on your own after this installation process...")
				} else {
					err := os.Chown(ctx.Config.ProperDatabasePath(), uid, gid)
					if err != nil {
						fmt.Fprintf(w, "<p class=\"warning\">Failed to chown sqlite database: %s. You need to perform this action on your own after this installation process...", err.Error())
					}
				}
			}
			if ctx.Config.Session.Type == "sqlite" {
				if gitUser == nil {
					fmt.Fprintf(w, "<p class=\"warning\">Failed to fild Git user's uid & gid when chowning sqlite database. You need to perform this action on your own after this installation process...")
				} else {
					err := os.Chown(ctx.Config.ProperSessionPath(), uid, gid)
					if err != nil {
						fmt.Fprintf(w, "<p class=\"warning\">Failed to chown sqlite database: %s. You need to perform this action on your own after this installation process...", err.Error())
					}
				}
			}
			if ctx.Config.ReceiptSystem.Type == "sqlite" {
				if gitUser == nil {
					fmt.Fprintf(w, "<p class=\"warning\">Failed to fild Git user's uid & gid when chowning sqlite database. You need to perform this action on your own after this installation process...")
				} else {
					err := os.Chown(ctx.Config.ProperReceiptSystemPath(), uid, gid)
					if err != nil {
						fmt.Fprintf(w, "<p class=\"warning\">Failed to chown sqlite database: %s. You need to perform this action on your own after this installation process...", err.Error())
					}
				}
			}
			ctx.GitUserHome = gitUser.HomeDir
			return true
		}() { goto leave }

		if ctx.Config.OperationMode == gitus.OP_MODE_SIMPLE {
			if !func()bool{
				var nsName string
				var keyRepoRelPath, configRepoRelPath string
				if ctx.Config.UseNamespace {
					nsName = "__gitus"
					keyRepoRelPath = path.Join(nsName, "__keys")
					configRepoRelPath = path.Join(nsName, "__repo_config")
				} else {
					keyRepoRelPath = "__keys"
					configRepoRelPath = "__repo_config"
				}
				keyRepoFullPath := path.Join(ctx.Config.GitRoot, keyRepoRelPath)
				configRepoFullPath := path.Join(ctx.Config.GitRoot, configRepoRelPath)
				cu, _ := user.Current()

				// make sure this path is absolute.  this is for
				// setting up update hook for key repo and config
				// repo.
				configFullPath := ctx.Config.FilePath
				if !path.IsAbs(configFullPath) {
					configFullPath = path.Clean(path.Join(ctx.GitUserHome, configFullPath))
				}
				
				// setting up key repo
				keyRepo, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, nsName, "__keys", keyRepoFullPath)
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to create key repository</p>")
					return false
				}
				// we must make sure we own the repo before adding file...
				err = model.ChangeFileSystemOwner(keyRepo, cu)
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to obtain key repository ownership for setting it up: %s</p>", err)
					return false
				}
				_, err = model.AddFileToRepoString(keyRepo, "master", "admin/ssh/master_key", "Gitus Web Installer", "gitus@web.installer", "Gitus Web Installer", "gitus@web.installer", "init", ctx.RootSSHKey)
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to add root ssh key to key repository: %s</p>", err)
					return false
				}
				// setting up hook.
				keyGitRepo := keyRepo.(*gitlib.LocalGitRepository)
				err = keyGitRepo.SaveHook("update", fmt.Sprintf(`
#!/bin/sh

# --- Command line
refname="$1"
oldrev="$2"
newrev="$3"

# --- Safety check
if [ -z "$GIT_DIR" ]; then
	echo "Don't run this script from the command line." >&2
	echo " (if you want, you could supply GIT_DIR then run" >&2
	echo "  $0 <ref> <oldrev> <newrev>)" >&2
	exit 1
fi

if [ -z "$refname" -o -z "$oldrev" -o -z "$newrev" ]; then
	echo "usage: $0 <ref> <oldrev> <newrev>" >&2
	exit 1
fi

# --- Config
allowunannotated=$(git config --type=bool hooks.allowunannotated)
allowdeletebranch=$(git config --type=bool hooks.allowdeletebranch)
denycreatebranch=$(git config --type=bool hooks.denycreatebranch)
allowdeletetag=$(git config --type=bool hooks.allowdeletetag)
allowmodifytag=$(git config --type=bool hooks.allowmodifytag)

# --- Check types
# if $newrev is 0000...0000, it's a commit to delete a ref.
zero=$(git hash-object --stdin </dev/null | tr '[0-9a-f]' '0')
if [ "$newrev" = "$zero" ]; then
	newrev_type=delete
else
	newrev_type=$(git cat-file -t $newrev)
fi

  case "$refname","$newrev_type" in
	refs/tags/*,commit)
		# un-annotated tag
		short_refname=${refname##refs/tags/}
		if [ "$allowunannotated" != "true" ]; then
			echo "*** The un-annotated tag, $short_refname, is not allowed in this repository" >&2
			echo "*** Use 'git tag [ -a | -s ]' for tags you want to propagate." >&2
			exit 1
		fi
		;;
	refs/tags/*,delete)
		# delete tag
		if [ "$allowdeletetag" != "true" ]; then
			echo "*** Deleting a tag is not allowed in this repository" >&2
			exit 1
		fi
		;;
	refs/tags/*,tag)
		# annotated tag
		if [ "$allowmodifytag" != "true" ] && git rev-parse $refname > /dev/null 2>&1
		then
			echo "*** Tag '$refname' already exists." >&2
			echo "*** Modifying a tag is not allowed in this repository." >&2
			exit 1
		fi
		;;
	refs/heads/*,commit)
		# branch
		if [ "$oldrev" = "$zero" -a "$denycreatebranch" = "true" ]; then
			echo "*** Creating a branch is not allowed in this repository" >&2
			exit 1
		else
            if [ "$refname" = "refs/heads/master" ]; then
    			%s -config "%s" simple-mode keys-update "$newrev"
            fi
		fi
		;;
    refs/heads/*,delete)
		# delete branch
		if [ "$allowdeletebranch" != "true" ]; then
			echo "*** Deleting a branch is not allowed in this repository" >&2
			exit 1
		fi
		;;
	refs/remotes/*,commit)
		# tracking branch
		;;
	refs/remotes/*,delete)
		# delete tracking branch
		if [ "$allowdeletebranch" != "true" ]; then
			echo "*** Deleting a tracking branch is not allowed in this repository" >&2
			exit 1
		fi
 		;;
	,*)
		# Anything else (is there anything else?)
		echo "*** Update hook: unknown type of update to ref $refname of type $newrev_type" >&2
		exit 1
		;;
esac

# --- Finished
exit 0
`, path.Join(ctx.GitUserHome, "git-shell-commands", "gitus"), shellparse.Quote(configFullPath)))
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to setup git update hook for key repository: %s</p>", err)
					return false
				}
				err = model.ChangeFileSystemOwnerByName(keyRepo, ctx.Config.GitUser)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to return the key repo to configured git user: %s.</p>", err)
					return false
				}

				// setting up config repo.
				fileList := make(map[string]string, 0)
				configRepo, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, nsName, "__repo_config", configRepoFullPath)
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to setup config repository properly: %s</p>", err)
					return false
				}
				err = model.ChangeFileSystemOwner(configRepo, cu)
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to change config repo owner: %s</p>", err)
					return false
				}
				if ctx.Config.UseNamespace {
					fileList["__gitus/config.json"] = `{
    "namespace": {
        "description": "",
        "visibility": "private"
    }
}
`
				}
				fileList[path.Join(keyRepoRelPath, "config.json")] = `{ 
    "repo": {
        "description": "",
        "visibility": "private"
    },
    "hooks": {
    },
    "users": {
        "admin": {
            "default": "allow"
        }
    }
}
`
				fileList[path.Join(configRepoRelPath, "config.json")] = `{
    "repo": {
        "description": "",
        "visibility": "private"
    },
    "hooks": {
    },
    "users": {
        "admin": {
            "default": "allow"
        }
    }
}
`
				_, err = model.AddMultipleFileToRepoString(configRepo, "master", "Gitus Web Installer", "gitus@web.installer", "Gitus Web Installer", "gitus@web.installer", "init", fileList)
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to add commit to config repository: %s</p>", err)
					return false
				}

				err = configRepo.(*gitlib.LocalGitRepository).SaveHook("post-update", fmt.Sprintf(`
#!/bin/sh

%s -config "%s" simple-mode gitus-sync "%s"
`, path.Join(ctx.GitUserHome, "git-shell-commands", "gitus"),
					shellparse.Quote(configFullPath),
					shellparse.Quote(path.Join(model.GetLocalRepositoryLocalPath(configRepo), "gitus_sync")),
				))
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to setup git post-update hook for config repo: %s</p>", err)
					return false
				}

				// setting up gitus_sync.  for the reason why
				// gitus_sync exists, see docs/simple-mode.org.
				cmd := exec.Command("git", "clone", ".", "gitus_sync")
				cmd.Dir = configRepoFullPath
				err = cmd.Run()
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to setup gitus_sync: %s</p>", err)
					return false
				}
				err = model.ChangeFileSystemOwnerByName(configRepo, ctx.Config.GitUser)
				if err != nil {
					fmt.Fprintf(w, "<p class=\"error\">Failed to return the config repo to configured git user: %s</p>", err)
					return false
				}
				
				if ctx.Config.UseNamespace {
					err = auxfuncs.ChangeLocationOwnerByName(path.Join(ctx.Config.GitRoot, "__gitus"), ctx.Config.GitUser)
					if err != nil {
						fmt.Fprintf(w, "<p class=\"error\">Failed to return the namespace to configured git user: %s</p>", err)
						return false
					}
				}

				// setting up authorized_keys file
				authorizedKeysPath := path.Join(ctx.GitUserHome, ".ssh", "authorized_keys")
				keyEntry := fmt.Sprintf("command=\"gitus -config %s ssh admin master_key\" %s", shellparse.Quote(configFullPath), ctx.RootSSHKey)
				keyFile, err := os.OpenFile(authorizedKeysPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to create authorized_keys file: %s</p>", err)
					return false
				}
				_, err = fmt.Fprint(keyFile, keyEntry)
				if err != nil {
					fmt.Fprintf(w, "<p>Failed to write authorized_keys file: %s</p>", err)
					return false
				}
				keyFile.Close()
				fmt.Fprint(w, "<p><code>authorized_keys</code> file created. </p>")
				return true
			}() { goto leave }
		}
		
		fmt.Fprint(w, "<p>Done! <a href=\"./finish\">Go to the next step.</a></p>")
		goto footer

	leave:
		fmt.Fprintf(w, "<p>The installation process failed but the config file might've been saved successfully at <code>%s</code>. In this case, you need to run the following command:</p><pre>gitus -config %s</pre></p>", ctx.Config.FilePath, ctx.Config.FilePath)

	footer:
		fmt.Fprint(w, `
    <hr />
    <footer>
      <div class="footer-message">
        Powered by <a href="https://github.com/GitusCodeForge/Gitus">Gitus</a>.
      </div>
    </footer>
  </body>
</html>`)
	}))
	
	http.HandleFunc("GET /finish", withLog(func(w http.ResponseWriter, r *http.Request) {
		
		logTemplateError(ctx.loadTemplate("webinstaller/finish").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
		}))
	}))
}

func WebInstaller() {
	fmt.Println("This is the Gitus web installer. We will start a web server, which allows us to provide you a more user-friendly interface for configuring your Gitus instance. This web server will be shut down when the installation is finished. You can always start the web installer by using the `-init` flag or the `install` command.")
	var portNum int = 0
	for {
		r, err := askString("Please enter the port number this web server would bind to.", "8001")
		if err != nil {
			fmt.Printf("Failed to get a response: %s\n", err.Error())
			os.Exit(1)
		}
		portNum, err = strconv.Atoi(strings.TrimSpace(r))
		if err == nil { break }
		fmt.Println("Please enter a valid number...")
	}
	masterTemplate := templates.LoadTemplate()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	server := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%d", portNum),
	}
	bindAllWebInstallerRoutes(&WebInstallerRoutingContext{
		Template: masterTemplate,
		Config: &gitus.GitusConfig{},
	})
	go func() {
		log.Printf("Trying to serve at %s:%d\n", "0.0.0.0", portNum)
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections.")
	}()

	<-sigChan
	
	if err := server.Shutdown(context.TODO()); err != nil {
		log.Fatalf("HTTP shutdown fail: %v", err)
	}
	
	log.Println("Graceful shutdown complete.")
}

