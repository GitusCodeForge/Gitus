package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/golang-jwt/jwt/v5"
)

var REGEX_BEARER regexp.Regexp = *regexp.MustCompile(`\s*Bearer\*webhook-jwt-(.*)\s*`)

func bindWebhookResultReportController(ctx *RouterContext) {
	http.HandleFunc("POST /webhook-result-report", UseMiddleware(
		[]Middleware{Logged, JSONRequestRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			body := model.WebhookResult{}
			b := new(bytes.Buffer)
			_, err := io.Copy(b, r.Body)
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Failed to read request body: %s", err)
				return
			}
			err = json.Unmarshal(b.Bytes(), &body)
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Failed to unmarshal body: %s", err)
				return
			}
			ogRes, err := rc.DatabaseInterface.GetWebhookResultByUUID(body.UUID)
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Failed to find corresponding entry: %s", err)
				return
			}
			repo, err := rc.DatabaseInterface.GetRepositoryByName(ogRes.RepoNamespace, ogRes.RepoName)
			if errors.Is(err, db.ErrEntityNotFound) {
				w.WriteHeader(404)
				fmt.Fprintf(w, "Not found: %s", err)
				return
			}
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Failed to find corresponding entry: %s", err)
				return
			}
			bearer := r.Header.Get("Authorization")
			p := REGEX_BEARER.FindStringSubmatch(bearer)
			if len(p) <= 0 {
				w.WriteHeader(400)
				fmt.Fprintf(w, "Failed to verify JWT: %s", err)
				return
			}
			if repo.WebHookConfig.Secret == "" {
				w.WriteHeader(400)
				fmt.Fprintf(w, "Empty secret is not allowed; please check your repository config.")
				return
			}
			token, err := jwt.Parse(p[1], func(token *jwt.Token) (any, error) {
				return repo.WebHookConfig.Secret, nil
			}, jwt.WithValidMethods([]string{jwt.SigningMethodHS512.Alg()}))
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Failed to validate JWT: %s", err)
				return
			}
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if ogRes.ReportUUID == claims["jti"] && ogRes.Status == model.WEBHOOK_RESULT_UNDEFINED {
					ogRes.Status = body.Status
					ogRes.Message = body.Message
					ogRes.Timestamp = time.Now().Unix()
					err = rc.DatabaseInterface.UpdateWebhookResult(body.UUID, ogRes)
					if err != nil {
						w.WriteHeader(500)
						fmt.Fprintf(w, "Failed to update: %s", err)
						return
					}
				} else {
					w.WriteHeader(500)
					fmt.Fprintf(w, "Failed to validate JWT: %s", err)
					return
				}
			} else {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Failed to validate JWT: %s", err)
				return
			}
			w.WriteHeader(200)
			fmt.Fprint(w, "OK")
		},
	))
}

