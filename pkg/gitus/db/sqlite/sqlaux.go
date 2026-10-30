package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

// Column lists shared by every listing query of the same entity. Keeping them
// in one place means the SELECT clause and the scan function below cannot
// drift apart.
const (
	repositoryColumns = `repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, rowid`
	namespaceColumns = `ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl`
	issueColumns = `rowid, issue_id, issue_author, issue_status, issue_title, issue_content, issue_timestamp, issue_priority`
	pullRequestColumns = `rowid, pull_request_id, username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp`
)

// Repository statuses: 1 public, 2 private, 4 archived, 5 internal, 6 limited.
// Namespace statuses: 1 public, 2 private, 3 internal.
//
// "archived" stays readable and "internal" is what a logged in user gets on
// top of the public part; a guest sees public (and archived) only.
const (
	repoCondPublic = `(repo_status = 1 OR repo_status = 4)`
	repoCondInternal = `(repo_status = 1 OR repo_status = 4 OR repo_status = 5)`
	nsCondPublic = `(ns_status = 1)`
	nsCondInternal = `(ns_status = 1 OR ns_status = 3)`
)

func (dbif *SqliteGitusDatabaseInterface) queryRows(q string, args ...any) (*sql.Rows, error) {
	stmt, err := dbif.prepare(q)
	if err != nil { return nil, err }
	return stmt.Query(args...)
}

func (dbif *SqliteGitusDatabaseInterface) countQuery(q string, args ...any) (int64, error) {
	stmt, err := dbif.prepare(q)
	if err != nil { return 0, err }
	var n int64
	err = stmt.QueryRow(args...).Scan(&n)
	if err != nil { return 0, err }
	return n, nil
}

// repoViewerCond matches repositories the given user owns or was granted
// access to through the repository ACL. As on the repository pages
// themselves, an ACL entry on the residing namespace counts as well.
func (dbif *SqliteGitusDatabaseInterface) repoViewerCond(b *db.QueryBuilder, viewer string) string {
	pat := db.ToSqlSearchPattern(viewer)
	direct := fmt.Sprintf("(repo_owner = %s OR repo_acl LIKE %s ESCAPE %s)",
		b.Arg(viewer), b.Arg(pat), b.Arg("\\"))
	viaNamespace := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM %s_namespace ns WHERE ns.ns_name = repo.repo_namespace AND (ns_owner = %s OR ns_acl LIKE %s ESCAPE %s))",
		dbif.config.Database.TablePrefix, b.Arg(viewer), b.Arg(pat), b.Arg("\\"))
	return fmt.Sprintf("(%s OR %s)", direct, viaNamespace)
}

// nsViewerCond matches namespaces the given user owns or was granted access
// to through the ACL.
func (dbif *SqliteGitusDatabaseInterface) nsViewerCond(b *db.QueryBuilder, viewer string) string {
	pat := db.ToSqlSearchPattern(viewer)
	return fmt.Sprintf("(ns_owner = %s OR ns_acl LIKE %s ESCAPE %s)",
		b.Arg(viewer), b.Arg(pat), b.Arg("\\"))
}

func repoKeywordCond(b *db.QueryBuilder, q string) string {
	if len(q) <= 0 { return "" }
	pat := db.ToSqlSearchPattern(q)
	return fmt.Sprintf("(repo_namespace LIKE %s ESCAPE %s OR repo_name LIKE %s ESCAPE %s)",
		b.Arg(pat), b.Arg("\\"), b.Arg(pat), b.Arg("\\"))
}

func nsKeywordCond(b *db.QueryBuilder, q string) string {
	if len(q) <= 0 { return "" }
	pat := db.ToSqlSearchPattern(q)
	return fmt.Sprintf("(ns_name LIKE %s ESCAPE %s OR ns_title LIKE %s ESCAPE %s)",
		b.Arg(pat), b.Arg("\\"), b.Arg(pat), b.Arg("\\"))
}

func issueKeywordCond(b *db.QueryBuilder, q string) string {
	if len(q) <= 0 { return "" }
	pat := db.ToSqlSearchPattern(q)
	return fmt.Sprintf("issue_title LIKE %s ESCAPE %s", b.Arg(pat), b.Arg("\\"))
}

func pullRequestKeywordCond(b *db.QueryBuilder, q string) string {
	if len(q) <= 0 { return "" }
	pat := db.ToSqlSearchPattern(q)
	return fmt.Sprintf("title LIKE %s ESCAPE %s", b.Arg(pat), b.Arg("\\"))
}

func repoLabelCond(b *db.QueryBuilder, label string) string {
	pat := db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label))
	return fmt.Sprintf("repo_label_list LIKE %s ESCAPE %s", b.Arg(pat), b.Arg("\\"))
}

func issueStatusCond(filterType int) string {
	switch filterType {
	case 1: return "issue_status = 1"
	case 2: return "NOT (issue_status = 1)"
	case 3: return "issue_status = 2"
	case 4: return "issue_status = 3"
	}
	return ""
}

func pullRequestStatusCond(filterType int) string {
	switch filterType {
	case 1: return "pull_request_status = 1"
	case 2: return "NOT (pull_request_status = 1)"
	case 3: return "pull_request_status = 2"
	case 4: return "pull_request_status = 3"
	}
	return ""
}

// repoVisibilityCond is the single definition of "a repository the viewer may
// see". It mirrors what the repository pages themselves check: private
// repositories are only listed for their owner and for users the ACL grants
// access to; "archived" stays readable; "internal" is what a logged in user
// gets on top of the public part.
//
// The status of the owning namespace deliberately plays no role here: a
// namespace only controls whether the namespace itself is listed, while
// access to a repository is decided by the repository status and ACL.
func (dbif *SqliteGitusDatabaseInterface) repoVisibilityCond(b *db.QueryBuilder, viewer string) string {
	if len(viewer) <= 0 { return repoCondPublic }
	return fmt.Sprintf("(%s OR %s)", repoCondInternal, dbif.repoViewerCond(b, viewer))
}

// nsVisibilityCond is the single definition of "a namespace the viewer may
// see".
func (dbif *SqliteGitusDatabaseInterface) nsVisibilityCond(b *db.QueryBuilder, viewer string) string {
	if len(viewer) <= 0 { return nsCondPublic }
	return fmt.Sprintf("(%s OR %s)", nsCondInternal, dbif.nsViewerCond(b, viewer))
}

// belongingRepoCond is what a user profile page uses: repositories of `user`
// as far as `viewingUser` may see them.
func (dbif *SqliteGitusDatabaseInterface) belongingRepoCond(b *db.QueryBuilder, viewingUser string, user string) string {
	owned := dbif.repoViewerCond(b, user)
	if len(viewingUser) <= 0 {
		return fmt.Sprintf("(%s AND %s)", repoCondPublic, owned)
	}
	if viewingUser == user {
		return fmt.Sprintf("(%s)", owned)
	}
	return fmt.Sprintf("(%s AND (%s OR %s))", owned, repoCondInternal, dbif.repoViewerCond(b, viewingUser))
}

// belongingNamespaceCond is what a user profile page uses: namespaces of
// `user` as far as `viewingUser` may see them.
func (dbif *SqliteGitusDatabaseInterface) belongingNamespaceCond(b *db.QueryBuilder, viewingUser string, user string) string {
	owned := dbif.nsViewerCond(b, user)
	if len(viewingUser) <= 0 {
		return fmt.Sprintf("(%s AND %s)", nsCondPublic, owned)
	}
	if viewingUser == user {
		return fmt.Sprintf("(%s)", owned)
	}
	return fmt.Sprintf("(%s AND (%s OR %s))", owned, nsCondInternal, dbif.nsViewerCond(b, viewingUser))
}

// buildRepository turns one row of repositoryColumns into a model. It is shared
// by the plain listing scanner and by GetRepositoryWithNamespace, which reads a
// repository and its namespace in a single query.
func (dbif *SqliteGitusDatabaseInterface) buildRepository(repoType uint8, ns, name, desc, owner, acl string, status int64, forkOriginNs, forkOriginName, labelList, webhookStr string, rowid int64) (*model.Repository, error) {
	a, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	p := path.Join(dbif.config.GitRoot, ns, name)
	lr, err := model.CreateLocalRepository(repoType, ns, name, p)
	if err != nil { return nil, err }
	var tags []string = nil
	if len(labelList) > 0 {
		tags = strings.Split(labelList[1:len(labelList)-1], "}{")
	}
	webhookObj, err := model.ParseWebHookConfig(webhookStr)
	if err != nil { return nil, err }
	return &model.Repository{
		AbsId: rowid,
		Type: repoType,
		Namespace: ns,
		Name: name,
		Owner: owner,
		Description: desc,
		AccessControlList: a,
		Status: model.GitusRepositoryStatus(status),
		Repository: lr,
		ForkOriginNamespace: forkOriginNs,
		ForkOriginName: forkOriginName,
		RepoLabelList: tags,
		WebHookConfig: webhookObj,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) scanRepositoryRows(r *sql.Rows) ([]*model.Repository, error) {
	res := make([]*model.Repository, 0)
	for r.Next() {
		var repoType uint8
		var ns, name, desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookStr string
		var status, rowid int64
		err := r.Scan(&repoType, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName, &labelList, &webhookStr, &rowid)
		if err != nil { return nil, err }
		repo, err := dbif.buildRepository(repoType, ns, name, desc, owner, acl, status, forkOriginNs, forkOriginName, labelList, webhookStr, rowid)
		if err != nil { return nil, err }
		res = append(res, repo)
	}
	if r.Err() != nil { return nil, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) scanNamespaceRows(r *sql.Rows) (map[string]*model.Namespace, error) {
	list, err := dbif.scanNamespaceList(r)
	if err != nil { return nil, err }
	res := make(map[string]*model.Namespace, len(list))
	for _, ns := range list {
		res[ns.Name] = ns
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) buildNamespace(name, title, desc, email, owner string, regTime int64, status int64, acl string) (*model.Namespace, error) {
	a, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	return &model.Namespace{
		Name: name,
		Title: title,
		Description: desc,
		Email: email,
		Owner: owner,
		RegisterTime: regTime,
		ACL: a,
		Status: model.GitusNamespaceStatus(status),
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) scanNamespaceList(r *sql.Rows) ([]*model.Namespace, error) {
	res := make([]*model.Namespace, 0)
	for r.Next() {
		var name, title, desc, email, owner, acl string
		var regTime int64
		var status int64
		err := r.Scan(&name, &title, &desc, &email, &owner, &regTime, &status, &acl)
		if err != nil { return nil, err }
		ns, err := dbif.buildNamespace(name, title, desc, email, owner, regTime, status, acl)
		if err != nil { return nil, err }
		res = append(res, ns)
	}
	if r.Err() != nil { return nil, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) scanIssueRows(r *sql.Rows, ns string, name string) ([]*model.Issue, error) {
	res := make([]*model.Issue, 0)
	for r.Next() {
		var issueAbsId, issueTimestamp int64
		var issueId, issueStatus, issuePriority int
		var issueAuthor, issueTitle, issueContent string
		err := r.Scan(&issueAbsId, &issueId, &issueAuthor, &issueStatus, &issueTitle, &issueContent, &issueTimestamp, &issuePriority)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: issueAbsId,
			RepoNamespace: ns,
			RepoName: name,
			IssueStatus: issueStatus,
			IssueId: issueId,
			IssueTime: issueTimestamp,
			IssueTitle: issueTitle,
			IssueAuthor: issueAuthor,
			IssueContent: issueContent,
			IssuePriority: issuePriority,
		})
	}
	if r.Err() != nil { return nil, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) scanPullRequestRows(r *sql.Rows, ns string, name string) ([]*model.PullRequest, error) {
	res := make([]*model.PullRequest, 0)
	for r.Next() {
		var prId, absId, prTime, mergeCheckTimestamp int64
		var status int
		var author, title, receiverBranch string
		var providerNamespace, providerName, providerBranch string
		var mergeCheckResultStr string
		err := r.Scan(&absId, &prId, &author, &title, &receiverBranch, &providerNamespace, &providerName, &providerBranch, &mergeCheckResultStr, &mergeCheckTimestamp, &status, &prTime)
		if err != nil { return nil, err }
		var mergeCheckResult *gitlib.MergeCheckResult = nil
		if len(mergeCheckResultStr) > 0 {
			err = json.Unmarshal([]byte(mergeCheckResultStr), &mergeCheckResult)
			if err != nil { return nil, err }
		}
		res = append(res, &model.PullRequest{
			PRId: prId,
			PRAbsId: absId,
			Title: title,
			Author: author,
			Timestamp: prTime,
			ReceiverNamespace: ns,
			ReceiverName: name,
			ReceiverBranch: receiverBranch,
			ProviderNamespace: providerNamespace,
			ProviderName: providerName,
			ProviderBranch: providerBranch,
			Status: status,
			MergeCheckResult: mergeCheckResult,
			MergeCheckTimestamp: mergeCheckTimestamp,
		})
	}
	if r.Err() != nil { return nil, r.Err() }
	return res, nil
}
