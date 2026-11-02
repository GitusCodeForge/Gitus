package model

import (
	"errors"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

const (
	PULL_REQUEST_OPEN = 1
	PULL_REQUEST_CLOSED_AS_MERGED = 2
	PULL_REQUEST_CLOSED_AS_NOT_MERGED = 3
)

// Pull request review states. A reviewer is assigned as PENDING and either
// approves the current provider commit or asks for changes.
const (
	PULL_REQUEST_REVIEW_PENDING = 0
	PULL_REQUEST_REVIEW_APPROVED = 1
	PULL_REQUEST_REVIEW_CHANGES_REQUESTED = 2
)

// PullRequestReview is the current review of one reviewer. Only the current
// state is stored; the pull request page lists it next to the reviewer.
type PullRequestReview struct {
	PRAbsId int64
	Reviewer string
	Status int
	// ReviewedCommit is the provider branch tip the reviewer looked at. An
	// approval is only valid for that exact commit.
	ReviewedCommit string
	Timestamp int64
}

var (
	// ErrPullRequestReviewRequired means at least one assigned reviewer has not
	// approved the pull request yet (or asked for changes).
	ErrPullRequestReviewRequired = errors.New("the pull request needs approval from every reviewer")
	// ErrPullRequestReviewStale means an approval is for an older provider
	// commit; a new push has to be reviewed again.
	ErrPullRequestReviewStale = errors.New("the provider branch has new commits since the reviews; every reviewer must approve again")
)

type PullRequest struct {
	PRId int64
	PRAbsId int64
	Title string
	Author string
	Timestamp int64
	ReceiverNamespace string
	ReceiverName string
	ReceiverBranch string
	ProviderNamespace string
	ProviderName string
	ProviderBranch string
	Status int
	MergeCheckResult *gitlib.MergeCheckResult
	MergeCheckTimestamp int64
}

const (
	PULL_REQUEST_EVENT_COMMENT = 1
	PULL_REQUEST_EVENT_COMMENT_ON_CODE = 2
	PULL_REQUEST_EVENT_UPDATE_ON_BRANCH = 3
	PULL_REQUEST_EVENT_MERGE_CONFLICT_CHECK = 4
	PULL_REQUEST_EVENT_CLOSE_AS_NOT_MERGED = 5
	PULL_REQUEST_EVENT_CLOSE_AS_MERGED = 6
	PULL_REQUEST_EVENT_REOPEN = 7
)

type PullRequestEvent struct {
	PRAbsId int64
	// 1 - normal comment.
	// 2 - comment on code.
	// 3 - update on provider branch.
	// 4 - merge conflict check.
	// 5 - close as not merged.
	// 6 - close (merged).
	EventType int
	EventTimestamp int64
	EventAuthor string
	// type=1: string (comment text)
	// type=2: json dump of PullRequestCommentOnCode
	// type=3: string (commit id)
	// type=4: empty
	// type=5: empty
	// type=6: the merge method used ("merge", "squash" or "rebase")
	// type=7: empty
	EventContent string
}

type PullRequestComment struct {
	Content string `json:"content"`
}

type PullRequestCommentOnCode struct {
	RepoNamespace string `json:"repoNamespace"`
	RepoName string `json:"repoName"`
	CommitId string `json:"commitId"`
	Path string `json:"path"`
	LineRangeStart int `json:"lineRangeStart"`
	LineRangeEnd int `json:"lineRangeEnd"`
	Username string `json:"userName"`
	Content string `json:"content"`
	Code []string
}

type PullRequestUpdate struct {
	CommitId string `json:"commitId"`
}


