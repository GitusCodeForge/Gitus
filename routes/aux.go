package routes

import (
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/templates"
)

func LogIfError(err error) {
	if err != nil {
		log.Print(err.Error())
	}
}

const CSRF_KEY = "__csrf_token"

// go don't have ufcs so i'll have to suffer.
func WithLog(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf(" %s %s %s\n", r.RemoteAddr, r.Method, r.URL.Path)
		f(w, r)
	}
}
func WithLogHandler(f http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf(" %s %s %s\n", r.RemoteAddr, r.Method, r.URL.Path)
		f.ServeHTTP(w, r)
	}
}

func FoundAt(w http.ResponseWriter, p string) {
	w.Header().Add("Content-Length", "0")
	w.Header().Add("Location", p)
	w.WriteHeader(302)
}

func LoadTemplate(t *template.Template, name string) *template.Template {
	res := t.Lookup(name)
	if res == nil { log.Fatalf("Failed to find template \"%s\"", name) }
	return res
}

func LogTemplateError(e error) {
	if e != nil { log.Print(e) }
}

func GetUsernameFromCookie(r *http.Request) (string, error) {
	s, err := r.Cookie(COOKIE_KEY_USERNAME)
	if err != nil {
		return "", err
	} else {
		return s.Value, err
	}
}

func CheckUserSession(ctx *RouterContext, r *http.Request) (bool, error) {
	// the fact that go uses product type as disjoint union is
	// actually some ridiculously insane take. i have heard rumours
	// before starting this project, but to actually doing things this
	// way is a different story.
	un, err := GetUsernameFromCookie(r)
	if err == http.ErrNoCookie { return false, nil }
	if err != nil { return false, err }
	s, err := r.Cookie(COOKIE_KEY_SESSION)
	if err == http.ErrNoCookie { return false, nil }
	if err != nil { return false, err }
	res, err := ctx.SessionInterface.VerifySessionExist(un, s.Value)
	if err != nil { return false, err }
	return res, nil
}

func GenerateLoginInfoModel(ctx *RouterContext, r *http.Request) (*templates.LoginInfoModel, error) {
	// NOTE: we don't set .IsOwner here - that needs extra info and is
	// up to each controller.
	if ctx.Config.IsInPlainMode() { return nil, nil }
	loggedIn := false
	un, err := GetUsernameFromCookie(r)
	if err != nil {
		if err != http.ErrNoCookie { return nil, err }
		return &templates.LoginInfoModel{
			LoggedIn: loggedIn,
			UserName: "",
			UserFullName: "",
			UserEmail: "",
			UserSessionId: "",
			UserCSRFToken: "",
			IsOwner: false,
			IsSettingMember: false,
			IsAdmin: false,
			IsSuperAdmin: false,
		}, nil
	}
	s, err := r.Cookie(COOKIE_KEY_SESSION)
	if err != nil {
		if err != http.ErrNoCookie { return nil, err }
		return &templates.LoginInfoModel{
			LoggedIn: loggedIn,
			UserName: "",
			UserFullName: "",
			UserEmail: "",
			UserSessionId: "",
			UserCSRFToken: "",
			IsOwner: false,
			IsSettingMember: false,
			IsAdmin: false,
			IsSuperAdmin: false,
		}, nil
	}
	session, err := ctx.SessionInterface.RetrieveSessionByKey(un, s.Value)
	if err != nil { return nil, err }
	if session == nil {
		return &templates.LoginInfoModel{
			LoggedIn: loggedIn,
			UserName: "",
			UserFullName: "",
			UserEmail: "",
			UserSessionId: "",
			UserCSRFToken: "",
			IsOwner: false,
			IsSettingMember: false,
			IsAdmin: false,
			IsSuperAdmin: false,
		}, nil
	}
	u, err := ctx.DatabaseInterface.GetUserByName(un)
	if err != nil { return nil, err }
	return &templates.LoginInfoModel{
		LoggedIn: true,
		UserName: un,
		UserFullName: u.Title,
		UserEmail: u.Email,
		UserCSRFToken: session.CSRFToken,
		UserSessionId: session.Id,
		IsOwner: false,
		IsSettingMember: false,
		IsAdmin: u.Status == model.ADMIN || u.Status == model.SUPER_ADMIN,
		IsSuperAdmin: u.Status == model.SUPER_ADMIN,
	}, nil
}

func GenerateRepoHeader(typeStr string, nodeName string) *templates.RepoHeaderTemplateModel {
	repoHeaderInfo := &templates.RepoHeaderTemplateModel{
		TypeStr: typeStr,
		NodeName: nodeName,
	}
	return repoHeaderInfo
}

func GeneratePageInfo(r *http.Request, size int64) (*templates.PageInfoModel, error) {
	// set up a base pageInfo obj.  the TotalPage is left empty since
	// it depends on the result of actual query. a default of p=1 and
	// s=50 is set for the resulting value.  one shall change the
	// value of the resulting obj as needed. correction would be done
	// if a `size` of bigger or equal than 0 is specified, since by
	// our convention .PageNum starts from 1 and we need to restrict
	// its value within [1..totalPage]. if `size` is smaller than 0 ,
	// then this correction is not carried out.
	p := r.URL.Query().Get("p")
	if len(p) <= 0 { p = "1" }
	s := r.URL.Query().Get("s")
	if len(s) <= 0 { s = "50" }
	pageNum, err := strconv.ParseInt(p, 10, 64)
	if err != nil { return nil, err }
	pageSize, err := strconv.ParseInt(s, 10, 64)
	if err != nil { return nil, err }
	var totalPage int64 = 0
	if size == 0 {
		totalPage = 1
	} else if size > 0 {
		totalPage = size / pageSize
		if size % pageSize != 0 { totalPage += 1 }
		if pageNum <= 1 { pageNum = 1 }
		if pageNum > totalPage { pageNum = totalPage }
	}
	return &templates.PageInfoModel{
		PageNum: pageNum,
		PageSize: pageSize,
		TotalPage: totalPage,
	}, nil
}



