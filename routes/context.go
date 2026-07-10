package routes

// NOTE: this is NOT the context that's part of go's stdlib - it simply
// is a bunch of "misc supporting things" (e.g. templates & stuff) combined
// together and is not used to manage lifetimes & stuff AT ALL.

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/confirm_code"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/mail"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/ssh"
	"github.com/GitusCodeForge/Gitus/templates"
)

type RouterContext struct {
	GitUserHomeDirectory string
	Config *gitus.GitusConfig
	MasterTemplate *template.Template
	GitRepositoryList map[string]*model.Repository
	GitNamespaceList map[string]*model.Namespace
	DatabaseInterface db.GitusDatabaseInterface
	SessionInterface session.GitusSessionStore
	SSHKeyManagingContext *ssh.SSHKeyManagingContext
	ReceiptSystem receipt.GitusReceiptSystemInterface
	Mailer mail.GitusMailerInterface
	LoginInfo *templates.LoginInfoModel
	LastError error
	RateLimiter *RateLimiter
	ConfirmCodeManager confirm_code.GitusConfirmCodeManager
	SimpleModeConfigCache model.SimpleModeConfigCache
}

func (ctx RouterContext) LoadTemplate(name string) *template.Template {
	return ctx.MasterTemplate.Lookup(name)
}

func (ctx *RouterContext) NewLocal() *RouterContext {
	return &RouterContext{
		GitUserHomeDirectory: ctx.GitUserHomeDirectory,
		Config: ctx.Config,
		MasterTemplate: ctx.MasterTemplate,
		GitRepositoryList: ctx.GitRepositoryList,
		GitNamespaceList: ctx.GitNamespaceList,
		DatabaseInterface: ctx.DatabaseInterface,
		SessionInterface: ctx.SessionInterface,
		SSHKeyManagingContext: ctx.SSHKeyManagingContext,
		ReceiptSystem: ctx.ReceiptSystem,
		Mailer: ctx.Mailer,
		LoginInfo: &templates.LoginInfoModel{},
		LastError: ctx.LastError,
		RateLimiter: ctx.RateLimiter,
		ConfirmCodeManager: ctx.ConfirmCodeManager,
	}
}

func (ctx *RouterContext) ResolvePathAgainstHome(p string) string {
	if !path.IsAbs(p) {
		return path.Clean(path.Join(ctx.GitUserHomeDirectory, p))
	} else {
		return p
	}
}

func (ctx RouterContext) ReportNotFound(objName string, objType string, namespace string, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(404)
	LogTemplateError(ctx.LoadTemplate("error").Execute(w,
		templates.ErrorTemplateModel{
			Config: ctx.Config,
			ErrorCode: 404,
			ErrorMessage: fmt.Sprintf(
				"%s %s not found in %s",
				objType, objName, namespace,
			),
		},
	))
}

func (ctx RouterContext) ReportNormalError(msg string, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(400)
	LogTemplateError(ctx.LoadTemplate("error").Execute(w,
		templates.ErrorTemplateModel{
			Config: ctx.Config,
			ErrorCode: 400,
			ErrorMessage: fmt.Sprintf(
				"Error: %s",
				msg,
			),
		},
	))
}

func (ctx RouterContext) ReportInternalError(msg string, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(500)
	LogTemplateError(ctx.LoadTemplate("error").Execute(w,
		templates.ErrorTemplateModel{
			Config: ctx.Config,
			ErrorCode: 500,
			ErrorMessage: fmt.Sprintf(
				"Internal error: %s",
				msg,
			),
		},
	))
}

func (ctx RouterContext) ReportForbidden(msg string, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(403)
	LogTemplateError(ctx.LoadTemplate("error").Execute(w,
		templates.ErrorTemplateModel{
			Config: ctx.Config,
			ErrorCode: 403,
			ErrorMessage: fmt.Sprintf(
				"Forbidden: %s",
				msg,
			),
		},
	))
}

func (ctx RouterContext) ReportObjectReadFailure(objid string, msg string, w http.ResponseWriter, r *http.Request) {
	ctx.ReportInternalError(
		fmt.Sprintf(
			"Fail to read object %s: %s",
			objid, msg,
		), w, r,
	)
}

func (ctx RouterContext) ReportObjectTypeMismatch(objid string, expectedType string, actualType string, w http.ResponseWriter, r *http.Request) {
	ctx.ReportInternalError(
		fmt.Sprintf(
			"Object type mismatch for %s: %s expected but %s found",
			objid, expectedType, actualType,
		), w, r,
	)
}

func (ctx *RouterContext) ReportRedirect(target string, timeout int, title string, message string, w http.ResponseWriter, r *http.Request) {
	var loginInfoModel *templates.LoginInfoModel
	var err error
	if !ctx.Config.IsInPlainMode() {
		loginInfoModel, err = GenerateLoginInfoModel(ctx, r)
		if err != nil { panic(err) }
	}
	LogTemplateError(ctx.LoadTemplate("_redirect/index").Execute(w, templates.RedirectWithMessageModel{
		Config: ctx.Config,
		LoginInfo: loginInfoModel,
		Timeout: timeout,
		RedirectUrl: target,
		MessageTitle: title,
		MessageText: message,
	}))
}

func (ctx *RouterContext) ReportSingleButtonCallback(
	target string,
	actionTitle string,
	actionText string,
	buttonText string,
	accompanyingData map[string]string,
	w http.ResponseWriter,
	r *http.Request,
) {
	var loginInfoModel *templates.LoginInfoModel
	var err error
	if !ctx.Config.IsInPlainMode() {
		loginInfoModel, err = GenerateLoginInfoModel(ctx, r)
		if err != nil { panic(err) }
	}
	LogTemplateError(ctx.LoadTemplate("_single-button-callbackk/index").Execute(w, templates.SingleButtonCallbackModel{
		Config: ctx.Config,
		LoginInfo: loginInfoModel,
		TargetUrl: target,
		AccompanyingData: accompanyingData,
		ActionTitle: actionTitle,
		ActionText: actionText,
		ButtonText: buttonText,
	}))
}

func (ctx *RouterContext) SyncAllNamespacePlain() error {
	if ctx.Config.UseNamespace {
		rp, err := ctx.Config.GetAllNamespacePlain()
		if err != nil { return err }
		ctx.GitNamespaceList = rp
	} else {
		ns, err := model.NewNamespace("", ctx.Config.GitRoot)
		if err != nil { return err }
		if ctx.GitNamespaceList == nil {
			ctx.GitNamespaceList = make(map[string]*model.Namespace, 0)
		}
		ctx.GitNamespaceList[""] = ns
	}
	return nil
}

func (ctx *RouterContext) SyncNamespacePlain(ns *model.Namespace) error {
	a, err := ctx.Config.GetAllRepositoryByNamespacePlain(ns.Name)
	if err != nil { return err }
	ns.RepositoryList = a
	return nil
}

func ParseRepositoryFullName(str string) (string, string) {
	np := strings.Split(strings.TrimSpace(str), ":")
	namespaceName := ""
	repoName := ""
	if len(np) <= 1 {
		namespaceName = ""
		repoName = np[0]
	} else {
		namespaceName = np[0]
		repoName = np[1]
	}
	return namespaceName, repoName
}

var ErrNotFound = errors.New("Requested object not found")

// we need the namespace acl to supplement the repository acl in terms
// of business logic.
func (ctx *RouterContext) ResolveRepositoryFullName(str string) (string, string, *model.Namespace, *model.Repository, error) {
	np := strings.Split(strings.TrimSpace(str), ":")
	namespaceName := ""
	repoName := ""
	if len(np) <= 1 {
		namespaceName = ""
		repoName = np[0]
	} else {
		namespaceName = np[0]
		repoName = np[1]
	}
	var rp *model.Repository
	var ok bool
	var err error
	var ns *model.Namespace
	if ctx.Config.IsInPlainMode() || ctx.Config.OperationMode == gitus.OP_MODE_SIMPLE {
		ns, ok = ctx.GitNamespaceList[namespaceName]
		if !ok {
			err := ctx.SyncAllNamespacePlain()
			if err != nil { return "", "", nil, nil, err }
			ns, ok = ctx.GitNamespaceList[namespaceName]
			if !ok {
				return "", "", nil, nil, ErrNotFound
			}
		}
		err = ctx.SyncNamespacePlain(ns)
		if err != nil { return "", "", nil, nil, err }
		rp, ok = ns.RepositoryList[repoName]
		if !ok {
			return "", "", nil, nil, ErrNotFound
		}
	} else {
		ns, err = ctx.DatabaseInterface.GetNamespaceByName(namespaceName)
		if err != nil { return "", "", nil, nil, err }
		rp, err = ctx.DatabaseInterface.GetRepositoryByName(namespaceName, repoName)
		if err != nil { return "", "", nil, nil, err }
	}
	return namespaceName, repoName, ns, rp, nil
}

