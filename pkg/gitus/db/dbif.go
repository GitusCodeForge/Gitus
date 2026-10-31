package db

import (
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

type GitusDatabaseInterface interface {
	// we have to discern between "database unusable" and "error while detecting".
	IsDatabaseUsable() (bool, error)
	InstallTables() error
	// EnsureSchema creates tables added after the database was installed. It is
	// idempotent and called once at startup.
	EnsureSchema() error
	Dispose() error
	
	GetUserByName(name string) (*model.GitusUser, error)
	GetUserLoginInfo(name string) (*model.GitusUserLoginInfo, error)
	GetAllAuthKeyByUsername(name string) ([]model.GitusAuthKey, error)
	GetAuthKeyByName(userName string, keyName string) (*model.GitusAuthKey, error)
	RegisterAuthKey(username string, keyname string, keytext string) error
	UpdateAuthKey(username string, keyname string, keytext string) error
	RemoveAuthKey(username string, keyname string) error
	GetAllSignKeyByUsername(name string) ([]model.GitusSigningKey, error)
	GetSignKeyByName(userName string, keyName string) (*model.GitusSigningKey, error)
	UpdateSignKey(username string, keyname string, keytext string) error
	RegisterSignKey(username string, keyname string, keytext string) error
	RemoveSignKey(username string, keyname string) error
	GetNamespaceByName(name string) (*model.Namespace, error)
	GetRepositoryByName(nsName string, repoName string) (*model.Repository, error)
	GetRepositoryWithNamespace(nsName string, repoName string) (*model.Namespace, *model.Repository, error)
	GetAllNamespace() (map[string]*model.Namespace, error)
	GetAllVisibleNamespacePaginated(username string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error)
	SearchAllVisibleNamespacePaginated(username string, query string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error)
	GetAllVisibleRepositoryPaginated(username string, pageNum int64, pageSize int64) ([]*model.Repository, error)
	SearchAllVisibleRepositoryPaginated(username string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error)
	GetAllNamespaceByOwner(name string) (map[string]*model.Namespace, error)
	GetAllRepositoryFromNamespace(name string) (map[string]*model.Repository, error)
	GetAllVisibleRepositoryFromNamespace(username string, ns string) ([]*model.Repository, error)
	RegisterUser(name string, email string, passwordHash string, status model.GitusUserStatus) (*model.GitusUser, error)
	// update user info. NOTE THAT any implementers MUST update the
	// status field as well if that has changed.
	UpdateUserInfo(name string, uobj *model.GitusUser) error
	UpdateUserPassword(name string, newPasswordHash string) error
	// soft delete vs. hard delete:
	// "soft delete" mark the user as deleted.
	// "hard delete" removes the data from the database for good.
	HardDeleteUserByName(name string) error
	UpdateUserStatus(name string, newStatus model.GitusUserStatus) error
	RegisterNamespace(name string, ownerUsername string) (*model.Namespace, error)
	UpdateNamespaceInfo(name string, nsobj *model.Namespace) error
	UpdateNamespaceOwner(name string, newOwner string) error
	UpdateNamespaceStatus(name string, newStatus model.GitusNamespaceStatus) error
	// the implementer should remove the directory as well.
	HardDeleteNamespaceByName(name string) error
	// the implementer should create the git directory as well. common
	// method is to run "git init".
	CreateRepository(ns string, name string, repoType uint8, owner string) (*model.Repository, error)
	// same as `CreateRepository` but with related fields being set.
	SetUpCloneRepository(originNs string, originName string, targetNs string, targetName string, owner string) (*model.Repository, error)
	UpdateRepositoryInfo(ns string, name string, robj *model.Repository) error
	UpdateRepositoryStatus(ns string, name string, status model.GitusRepositoryStatus) error
	HardDeleteRepository(ns string, name string) error
	// TODO: gitus currently doesn't support moving.
	// we'll reconsider this when the appropriate time comes.
	// MoveRepository(oldNs string, oldName string, newNs string, newName string) error

	GetAllUsers(pageNum int64, pageSize int64) ([]*model.GitusUser, error)
	GetAllNamespaces(pageNum int64, pageSize int64) (map[string]*model.Namespace, error)
	GetAllRepositories(pageNum int64, pageSize int64) ([]*model.Repository, error)

	CountAllUser() (int64, error)
	CountAllNamespace() (int64, error)
	CountAllRepositories() (int64, error)
	CountAllRepositoriesSearchResult(q string) (int64, error)
	CountAllVisibleNamespace(username string) (int64, error)
	CountAllVisibleRepositories(username string) (int64, error)

	// search user name & title containing the string `k`, case
	// insensitive.
	SearchForUser(k string, pageNum int64, pageSize int64) ([]*model.GitusUser, error)

	// search namespce name & title containing the string `k`, case
	// insensitive.
	SearchForNamespace(k string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error)

	// search repo namespace name & repository name & title containing
	// the string `k`, case insensitive.
	SearchForRepository(k string, pageNum int64, pageSize int64) ([]*model.Repository, error)

	// set ACL as specified.  implementer should remove permissions of
	// `targetUserName` when `acl` is nil,
	SetNamespaceACL(nsName string, targetUserName string, acl *model.ACLTuple) error
	SetRepositoryACL(nsName string, repoName string, targetUserName string, acl *model.ACLTuple) error

	GetAllComprisingNamespace(username string) (map[string]*model.Namespace, error)
	
	CountAllVisibleNamespaceSearchResult(username string, pattern string) (int64, error)
	CountAllVisibleRepositoriesSearchResult(username string, pattern string) (int64, error)

	GetAllRepositoryIssue(ns string, name string) ([]*model.Issue, error)
	GetRepositoryIssue(ns string, name string, iid int) (*model.Issue, error)
	CountAllRepositoryIssue(ns string, name string) (int, error)
	// filterType: 0 - all, 1 - open, 2 - closed, 3 - solved, 4 - discarded
	// when query = "" it looks for all issue.
	CountIssue(query string, namespace string, name string, filterType int) (int64, error)
	SearchIssuePaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.Issue, error)
	// returns the issue_id of the new issue.
	NewRepositoryIssue(ns string, name string, author string, title string, content string) (int64, error)
	HardDeleteRepositoryIssue(ns string, name string, issueId int) error
	SetIssuePriority(ns string, name string, id int64, priority int) error
	GetAllIssueEvent(ns string, name string, issueId int) ([]*model.IssueEvent, error)
	NewRepositoryIssueEvent(ns string, name string, issueId int64, eType int, author string, content string) error
	HardDeleteRepositoryIssueEvent(eventAbsId int64) error

	// return all namespace that `viewingUser` is a member of
	GetAllBelongingNamespace(viewingUser string, user string) ([]*model.Namespace, error)
	// return all repository that `viewingUser` is a member of
	GetAllBelongingRepository(viewingUser string, user string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error)
	CountAllBelongingRepository(viewingUser string, user string, query string) (int64, error)

	// implementers can choose to return nil or empty slice if there
	// isn't any fork repo of the specified repo; the caller should
	// check for both.
	GetForkRepositoryOfUser(username string, originNamespace string, originName string) ([]*model.Repository, error)

	GetAllPullRequestPaginated(namespace string, name string, pageNum int64, pageSize int64) ([]*model.PullRequest, error)
	NewPullRequest(username string, title string, receiverNamespace string, receiverName string, receiverBranch string, providerNamespace string, providerName string, providerBranch string) (int64, error)
	GetPullRequest(namespace string, name string, id int64) (*model.PullRequest, error)
	GetPullRequestByAbsId(absId int64) (*model.PullRequest, error)
	CheckPullRequestMergeConflict(absId int64) (*gitlib.MergeCheckResult, error)
	DeletePullRequest(absId int64) error
	GetAllPullRequestEventPaginated(absId int64, pageNum int64, pageSize int64) ([]*model.PullRequestEvent, error)
	CheckAndMergePullRequest(absId int64, username string) error
	CommentOnPullRequest(absId int64, author string, content string) (*model.PullRequestEvent, error)
	CommentOnPullRequestCode(absId int64, comment *model.PullRequestCommentOnCode) (*model.PullRequestEvent, error)
	ClosePullRequestAsNotMerged(absid int64, author string) error
	ReopenPullRequest(absid int64, author string) error
	// filterType: 0 - all, 1 - open, 2 - closed, 3 - merged, 4 - discarded
	// when query = "" it looks for all pull request.
	CountPullRequest(query string, namespace string, name string, filterType int) (int64, error)
	SearchPullRequestPaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.PullRequest, error)

	GetAllRegisteredEmailOfUser(username string) ([]struct{Email string;Verified bool}, error)
	AddEmail(username string, email string) error
	VerifyRegisteredEmail(username string, email string) error
	DeleteRegisteredEmail(username string, email string) error
	CheckIfEmailVerified(username string, email string) (bool, error)
	ResolveEmailToUsername(email string) (string, error)
	ResolveMultipleEmailToUsername(emailList map[string]string) (map[string]string, error)

	InsertRegistrationRequest(username string, email string, passwordHash string, reason string) error
	GetRegistrationRequestPaginated(pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error)
	GetRequestOfUsernamePaginated(username string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error)
	
	// NOTE: implementer should perform the RegisterUser action in this method as well. (but not the RegisterNamespace!...)
	ApproveRegistrationRequest(absid int64) error
	DisapproveRegistrationRequest(absid int64) error
	CountRegistrationRequest(query string) (int64, error)
	SearchRegistrationRequestPaginated(query string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error)
	GetRegistrationRequestByAbsId(absid int64) (*model.RegistrationRequest, error)

	AddRepositoryLabel(ns string, name string, lbl string) error
	RemoveRepositoryLabel(ns string, name string, lbl string) error
	GetRepositoryLabel(ns string, name string) ([]string, error)

	// NOTE: the reason username is here is that searching by label is
	// logically similar w/ searching by keywords, which means that all
	// the logic like acl controlled visibility applies.
	CountRepositoryWithLabel(username string, label string) (int64, error)
	GetRepositoryWithLabelPaginated(username string, label string, pageNum int64, pageSize int64) ([]*model.Repository, error)

	// NOTE: implementers should return "empty" (i.e. without actual data) model only.
	NewSnippet(username string, name string, status uint8) (*model.Snippet, error)
	GetAllSnippet(username string) ([]*model.Snippet, error)
	CountAllVisibleSnippet(username string, viewingUser string, query string) (int64, error)
	GetAllVisibleSnippetPaginated(username string, viewingUser string, query string, pageNum int64, pageSize int64) ([]*model.Snippet, error)
	DeleteSnippet(username string, name string) error
	SaveSnippetInfo(m *model.Snippet) error
	GetSnippet(username string, name string) (*model.Snippet, error)

	RegisterWebhookRequest(uuid string, reportUuid string, repoNs string, repoName string, commitId string) error
	UpdateWebhookResult(uuid string, result *model.WebhookResult) error
	GetWebhookResultByUUID(uuid string) (*model.WebhookResult, error)
}


