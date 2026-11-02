package admin

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/newuser"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminSiteConfigController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/site-config", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
			}))
		},
	))

	http.HandleFunc("POST /admin/site-config", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rc.Config.LockForSync()
			defer rc.Config.Unlock()
			switch r.Form.Get("section") {
			case "web":
				rc.Config.HttpHostName = r.Form.Get("http-host-name")
				rc.Config.SshHostName = r.Form.Get("ssh-host-name")
				rc.Config.StaticAssetDirectory = r.Form.Get("static-asset-directory")
				rc.Config.BindAddress = r.Form.Get("bind-address")
				i, err := strconv.ParseInt(r.Form.Get("bind-port"), 10, 32)
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Error while parsing bind port: %s. ", err.Error()),
					}))
					return
				}
				rc.Config.BindPort = int(i)
				err = rc.Config.Sync()
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Error while parsing request: %s. Please contact site owner for this...", err.Error()),
					}))
					return
				}
				rc.ReportRedirect("/admin/site-config", 3, "Updated", "Your specifie config has been updated.", w, r)
			case "basic":
				rc.Config.DepotName = r.Form.Get("depot-name")
				rc.Config.UseNamespace = false
				if r.Form.Has("use-namespace") && r.Form.Get("use-namespace") == "on" {
					rc.Config.UseNamespace = true
				}
				rc.Config.AllowRegistration = false
				if r.Form.Has("allow-registration") && r.Form.Get("allow-registration") == "on" {
					rc.Config.AllowRegistration = true
				}
				rc.Config.EmailConfirmationRequired = false
				if r.Form.Has("email-confirmation-required") && r.Form.Get("email-confirmation-required") == "on" {
					rc.Config.EmailConfirmationRequired = true
				}
				rc.Config.ManualApproval = false
				if r.Form.Has("manual-approval") && r.Form.Get("manual-approval") == "on" {
					rc.Config.ManualApproval = true
				}
				us, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("default-new-user-status")), 10, 32)
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Invalid default new user status: %s.", err.Error()),
					}))
					return
				}
				newUserStatus := model.GitusUserStatus(us)
				if !newuser.ValidDefaultStatus(newUserStatus) {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: "The default new user status cannot hand out administrator privileges.",
					}))
					return
				}
				newUserNamespace := strings.TrimSpace(r.Form.Get("default-new-user-namespace"))
				if !rc.Config.UseNamespace {
					newUserNamespace = ""
				}
				if len(newUserNamespace) > 0 {
					if !model.ValidNamespaceName(newUserNamespace) {
						LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
							Config: rc.Config,
							LoginInfo: rc.LoginInfo,
							ErrorMsg: fmt.Sprintf("Invalid default new user namespace name: %q.", newUserNamespace),
						}))
						return
					}
					if _, err := rc.DatabaseInterface.GetNamespaceByName(newUserNamespace); err != nil {
						LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
							Config: rc.Config,
							LoginInfo: rc.LoginInfo,
							ErrorMsg: fmt.Sprintf("The default new user namespace %q does not exist or cannot be read: %s.", newUserNamespace, err.Error()),
						}))
						return
					}
				}
				rc.Config.DefaultNewUserStatus = newUserStatus
				rc.Config.DefaultNewUserNamespace = newUserNamespace
				rc.Config.DebugMode = false
				if r.Form.Has("debug-mode") && r.Form.Get("debug-mode") == "on" {
					rc.Config.DebugMode = true
				}
				rc.Config.RecalculateProperPath()
				err = rc.Config.Sync()
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Error while saving config: %s. Please contact site owner for this...", err.Error()),
					}))
					return
				}
				rc.ReportRedirect("/admin/site-config", 3, "Updated", "Your specifie config has been updated.", w, r)
			case "git":
				rc.Config.GitRoot = r.Form.Get("root")
				rc.Config.GitUser = r.Form.Get("git-user")
				rc.Config.GitConfig.HTTPCloneProtocol.V1Dumb = len(strings.TrimSpace(r.Form.Get("git-http-enable-v1dumb"))) > 0
				rc.Config.GitConfig.HTTPCloneProtocol.V2 = len(strings.TrimSpace(r.Form.Get("git-http-enable-v2"))) > 0
				rc.Config.NoInteractiveShellMessage = strings.TrimSpace(r.Form.Get("no-interactive-shell-message"))
				sizeLimit, err := auxfuncs.ParseSize(r.Form.Get("repository-size-limit"))
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Invalid repository size limit: %s. Use a number of bytes or a suffix such as M or GiB.", err.Error()),
					}))
					return
				}
				rc.Config.GitConfig.RepositorySizeLimit = sizeLimit
				err = rc.Config.Sync()
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Error while saving config: %s. Please contact site owner for this...", err.Error()),
					}))
					return
				}
				rc.ReportRedirect("/admin/site-config", 3, "Updated", "Your specifie config has been updated.", w, r)
			case "ssh-auth":
				mode := strings.TrimSpace(r.Form.Get("ssh-auth-mode"))
				if !gitus.ValidSSHAuthMode(mode) {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Unknown SSH authentication mode: %q.", mode),
					}))
					return
				}
				provider := strings.TrimSpace(r.Form.Get("ssh-auth-provider"))
				if !gitus.ValidSSHAuthProvider(provider) {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Unknown SSH authentication provider: %q.", provider),
					}))
					return
				}
				command := strings.TrimSpace(r.Form.Get("ssh-auth-command"))
				if provider == gitus.SSH_AUTH_PROVIDER_COMMAND && !filepath.IsAbs(command) {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: "The external SSH authentication command must be an absolute path.",
					}))
					return
				}
				timeout, err := strconv.Atoi(strings.TrimSpace(r.Form.Get("ssh-auth-timeout")))
				if err != nil || timeout <= 0 || timeout > 300 {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: "The provider timeout must be a number of seconds between 1 and 300.",
					}))
					return
				}
				rc.Config.SSHAuth.Mode = mode
				rc.Config.SSHAuth.Provider = provider
				rc.Config.SSHAuth.Command = command
				rc.Config.SSHAuth.Timeout = timeout
				err = rc.Config.Sync()
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Error while saving config: %s. Please contact site owner for this...", err.Error()),
					}))
					return
				}
				// Switching back to the file mode should populate the file from
				// the database instead of waiting for the next key change.
				if !rc.Config.SSHAuth.UsesAuthorizedKeysCommand() && rc.SSHKeyManagingContext != nil {
					keys, err := rc.DatabaseInterface.GetAllAuthKey()
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to read SSH keys: %s", err), w, r)
						return
					}
					rc.SSHKeyManagingContext.Managed = nil
					for _, k := range keys {
						rc.SSHKeyManagingContext.AddAuthorizedKey(k.UserName, k.KeyName, k.KeyText)
					}
					if err := rc.SSHKeyManagingContext.Sync(); err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to write authorized_keys: %s", err), w, r)
						return
					}
				}
				rc.ReportRedirect("/admin/site-config", 3, "Updated", "Your specifie config has been updated.", w, r)
			case "theme-config":
				rc.Config.Theme.ForegroundColor = strings.TrimSpace(r.Form.Get("foreground-color"))
				rc.Config.Theme.BackgroundColor = strings.TrimSpace(r.Form.Get("background-color"))
				err := rc.Config.Sync()
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Error while saving config: %s. Please contact site owner for this...", err.Error()),
					}))
					return
				}
				rc.ReportRedirect("/admin/site-config", 3, "Updated", "Your specifie config has been updated.", w, r)
				
			case "front-page":
				rc.Config.FrontPage.Type = r.Form.Get("front-page-type")
				rc.Config.FrontPage.Namespace = r.Form.Get("namespace")
				rc.Config.FrontPage.Repository = r.Form.Get("repository")
				rc.Config.FrontPage.FileContent = r.Form.Get("file-content")
				err := rc.Config.Sync()
				if err != nil {
					LogTemplateError(rc.LoadTemplate("admin/site-config").Execute(w, &templates.AdminConfigTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: fmt.Sprintf("Error while saving config: %s. Please contact site owner for this...", err.Error()),
					}))
					return
				}
				rc.ReportRedirect("/admin/site-config", 3, "Updated", "Your specifie config has been updated.", w, r)
			}
		},
	))
}

