package mysql

import (
	"database/sql"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

// Column lists shared by every listing query of the same entity. Keeping them
// in one place means the SELECT clause and the scan function below cannot
// drift apart.
const (
	repositoryColumns = `repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid`
	namespaceColumns = `ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status`
	issueColumns = `issue_absid, issue_id, issue_author, issue_status, issue_title, issue_content, issue_timestamp, issue_priority`
	pullRequestColumns = `pull_request_absid, pull_request_id, author_username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp`
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

func (dbif *MysqlGitusDatabaseInterface) countQuery(q string, args ...any) (int64, error) {
	var n int64
	err := dbif.queryRow(context.Background(), q, args...).Scan(&n)
	if err != nil { return 0, err }
	return n, nil
}

// aclMatchCond matches the user name inside the JSON acl object, which is
// stored under the "acl" key (see model.ACL).
func aclMatchCond(b *db.QueryBuilder, column string, viewer string) string {
	return fmt.Sprintf("JSON_CONTAINS_PATH(%s, 'one', CONCAT('$.acl.\"', %s, '\"'))", column, b.Arg(viewer))
}

// repoViewerCond matches repositories the given user owns or was granted
// access to through the repository ACL. As on the repository pages
// themselves, an ACL entry on the residing namespace counts as well.
func (dbif *MysqlGitusDatabaseInterface) repoViewerCond(b *db.QueryBuilder, viewer string) string {
	direct := fmt.Sprintf("(repo_owner = %s OR %s)", b.Arg(viewer), aclMatchCond(b, "repo_acl", viewer))
	viaNamespace := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM %s_namespace ns WHERE ns.ns_name = repo.repo_namespace AND (ns_owner = %s OR %s))",
		dbif.config.Database.TablePrefix, b.Arg(viewer), aclMatchCond(b, "ns_acl", viewer))
	return fmt.Sprintf("(%s OR %s)", direct, viaNamespace)
}

// nsViewerCond matches namespaces the given user owns or was granted access
// to through the ACL.
func (dbif *MysqlGitusDatabaseInterface) nsViewerCond(b *db.QueryBuilder, viewer string) string {
	return fmt.Sprintf("(ns_owner = %s OR %s)", b.Arg(viewer), aclMatchCond(b, "ns_acl", viewer))
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
func (dbif *MysqlGitusDatabaseInterface) repoVisibilityCond(b *db.QueryBuilder, viewer string) string {
	if len(viewer) <= 0 { return repoCondPublic }
	return fmt.Sprintf("(%s OR %s)", repoCondInternal, dbif.repoViewerCond(b, viewer))
}

// nsVisibilityCond is the single definition of "a namespace the viewer may
// see".
func (dbif *MysqlGitusDatabaseInterface) nsVisibilityCond(b *db.QueryBuilder, viewer string) string {
	if len(viewer) <= 0 { return nsCondPublic }
	return fmt.Sprintf("(%s OR %s)", nsCondInternal, dbif.nsViewerCond(b, viewer))
}

// belongingRepoCond is what a user profile page uses: repositories of `user`
// as far as `viewingUser` may see them.
func (dbif *MysqlGitusDatabaseInterface) belongingRepoCond(b *db.QueryBuilder, viewingUser string, user string) string {
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
func (dbif *MysqlGitusDatabaseInterface) belongingNamespaceCond(b *db.QueryBuilder, viewingUser string, user string) string {
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
func (dbif *MysqlGitusDatabaseInterface) buildRepository(repoType int, ns, name, desc, owner, acl string, status int, forkOriginNs, forkOriginName, labelList, webhookStr string, absId int64) (*model.Repository, error) {
	a, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	p := path.Join(dbif.config.GitRoot, ns, name)
	lr, err := model.CreateLocalRepository(uint8(repoType), ns, name, p)
	if err != nil { return nil, err }
	var tags []string = nil
	if len(labelList) > 0 {
		tags = strings.Split(labelList[1:len(labelList)-1], "}{")
	}
	webhookObj, err := model.ParseWebHookConfig(webhookStr)
	if err != nil { return nil, err }
	return &model.Repository{
		AbsId: absId,
		Type: uint8(repoType),
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

func (dbif *MysqlGitusDatabaseInterface) scanRepositoryRows(rows *sql.Rows) ([]*model.Repository, error) {
	res := make([]*model.Repository, 0)
	for rows.Next() {
		var repoType, status int
		var ns, name, desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookStr string
		var absId int64
		err := rows.Scan(&repoType, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName, &labelList, &webhookStr, &absId)
		if err != nil { return nil, err }
		repo, err := dbif.buildRepository(repoType, ns, name, desc, owner, acl, status, forkOriginNs, forkOriginName, labelList, webhookStr, absId)
		if err != nil { return nil, err }
		res = append(res, repo)
	}
	if rows.Err() != nil { return nil, rows.Err() }
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) scanNamespaceRows(rows *sql.Rows) (map[string]*model.Namespace, error) {
	list, err := dbif.scanNamespaceList(rows)
	if err != nil { return nil, err }
	res := make(map[string]*model.Namespace, len(list))
	for _, ns := range list {
		res[ns.Name] = ns
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) buildNamespace(name, title, desc, email, owner string, regTime time.Time, acl string, status int) (*model.Namespace, error) {
	a, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	return &model.Namespace{
		Name: name,
		Title: title,
		Description: desc,
		Email: email,
		Owner: owner,
		RegisterTime: regTime.Unix(),
		ACL: a,
		Status: model.GitusNamespaceStatus(status),
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) scanNamespaceList(rows *sql.Rows) ([]*model.Namespace, error) {
	res := make([]*model.Namespace, 0)
	for rows.Next() {
		var name, title, desc, email, owner, acl string
		var regTime time.Time
		var status int
		err := rows.Scan(&name, &title, &desc, &email, &owner, &regTime, &acl, &status)
		if err != nil { return nil, err }
		ns, err := dbif.buildNamespace(name, title, desc, email, owner, regTime, acl, status)
		if err != nil { return nil, err }
		res = append(res, ns)
	}
	if rows.Err() != nil { return nil, rows.Err() }
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) scanIssueRows(rows *sql.Rows, ns string, name string) ([]*model.Issue, error) {
	res := make([]*model.Issue, 0)
	for rows.Next() {
		var absId, issueId int64
		var ts time.Time
		var author, title, content string
		var status, priority int
		err := rows.Scan(&absId, &issueId, &author, &status, &title, &content, &ts, &priority)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: absId,
			RepoNamespace: ns,
			RepoName: name,
			IssueId: int(issueId),
			IssueAuthor: author,
			IssueTitle: title,
			IssueContent: content,
			IssueTime: ts.Unix(),
			IssueStatus: status,
			IssuePriority: priority,
		})
	}
	if rows.Err() != nil { return nil, rows.Err() }
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) scanPullRequestRows(rows *sql.Rows, ns string, name string) ([]*model.PullRequest, error) {
	res := make([]*model.PullRequest, 0)
	for rows.Next() {
		var absId, prId int64
		var author, title, receiverBranch, providerNs, providerName, providerBranch, mergeCheckStr string
		var mergeCheckTime sql.NullTime
		var prTime time.Time
		var status int
		err := rows.Scan(&absId, &prId, &author, &title, &receiverBranch, &providerNs, &providerName, &providerBranch, &mergeCheckStr, &mergeCheckTime, &status, &prTime)
		if err != nil { return nil, err }
		var mergeCheckResult *gitlib.MergeCheckResult = nil
		if len(mergeCheckStr) > 0 {
			err = json.Unmarshal([]byte(mergeCheckStr), &mergeCheckResult)
			if err != nil { return nil, err }
		}
		res = append(res, &model.PullRequest{
			PRId: prId,
			PRAbsId: absId,
			Title: title,
			Author: author,
			Timestamp: prTime.Unix(),
			ReceiverNamespace: ns,
			ReceiverName: name,
			ReceiverBranch: receiverBranch,
			ProviderNamespace: providerNs,
			ProviderName: providerName,
			ProviderBranch: providerBranch,
			Status: status,
			MergeCheckResult: mergeCheckResult,
			MergeCheckTimestamp: nullTimeUnix(mergeCheckTime),
		})
	}
	if rows.Err() != nil { return nil, rows.Err() }
	return res, nil
}
