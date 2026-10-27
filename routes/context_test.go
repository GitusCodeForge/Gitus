package routes

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/templates"
)

func TestReportInternalError(t *testing.T) {
	ctx := RouterContext{
		Config: &gitus.GitusConfig{},
		MasterTemplate: templates.LoadTemplate(),
	}
	secret := "open /srv/git/secret.git/config: permission denied"

	render := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		ctx.ReportInternalError(secret, rec, httptest.NewRequest("GET", "/somewhere", nil))
		return rec
	}

	// Debug mode off: the visitor gets a generic message with a reference, not
	// the error itself.
	rec := render()
	if rec.Code != 500 {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "/srv/git") || strings.Contains(body, "permission denied") {
		t.Fatalf("internal error message leaked:\n%s", body)
	}
	if !strings.Contains(body, "reference") {
		t.Fatalf("the generic message does not point at the log:\n%s", body)
	}

	// Debug mode on: the message is shown as it is.
	ctx.Config.DebugMode = true
	rec = render()
	if !strings.Contains(rec.Body.String(), "/srv/git/secret.git/config") {
		t.Fatalf("debug mode did not show the error message:\n%s", rec.Body.String())
	}
}

func TestNewErrorReference(t *testing.T) {
	a := newErrorReference()
	b := newErrorReference()
	if len(a) != 8 {
		t.Fatalf("unexpected reference %q", a)
	}
	if a == b {
		t.Fatalf("the same reference was handed out twice: %q", a)
	}
}
