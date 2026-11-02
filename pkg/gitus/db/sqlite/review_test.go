package sqlite

import (
	"errors"
	"path"
	"strings"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

func TestPullRequestReviewApproval(t *testing.T) {
	dbif, _, forkPath, absId := setupMergeFixture(t, "reviewed")
	tip := strings.TrimSpace(mergeTestGit(t, forkPath, "rev-parse", "master"))

	reviews, err := dbif.GetPullRequestReviews(absId)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 0 {
		t.Fatalf("a fresh pull request has reviews: %+v", reviews)
	}
	if _, err := dbif.CheckPullRequestMergeConflict(absId); err != nil {
		t.Fatal(err)
	}
	if err := dbif.AddPullRequestReviewer(absId, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := dbif.AddPullRequestReviewer(absId, "bob"); err != nil {
		t.Fatal(err)
	}
	reviews, err = dbif.GetPullRequestReviews(absId)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].Reviewer != "bob" || reviews[0].Status != model.PULL_REQUEST_REVIEW_PENDING {
		t.Fatalf("reviews after adding bob = %+v", reviews)
	}

	// a pending reviewer blocks the merge.
	err = dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE)
	if !errors.Is(err, model.ErrPullRequestReviewRequired) {
		t.Fatalf("merge with a pending reviewer returned %v", err)
	}

	// an approval for another commit is stale.
	if err := dbif.SubmitPullRequestReview(absId, "bob", model.PULL_REQUEST_REVIEW_APPROVED, "0000000000000000000000000000000000000000"); err != nil {
		t.Fatal(err)
	}
	err = dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE)
	if !errors.Is(err, model.ErrPullRequestReviewStale) {
		t.Fatalf("merge with a stale approval returned %v", err)
	}

	// the current commit approves it.
	if err := dbif.SubmitPullRequestReview(absId, "bob", model.PULL_REQUEST_REVIEW_APPROVED, tip); err != nil {
		t.Fatal(err)
	}
	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); err != nil {
		t.Fatalf("merge after approval: %s", err)
	}
	pr, err := dbif.GetPullRequestByAbsId(absId)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Status != model.PULL_REQUEST_CLOSED_AS_MERGED {
		t.Errorf("status = %d", pr.Status)
	}
}

func TestPullRequestReviewMultipleReviewers(t *testing.T) {
	dbif, _, forkPath, absId := setupMergeFixture(t, "multiple reviewers")
	tip := strings.TrimSpace(mergeTestGit(t, forkPath, "rev-parse", "master"))
	if err := dbif.AddPullRequestReviewer(absId, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := dbif.AddPullRequestReviewer(absId, "carol"); err != nil {
		t.Fatal(err)
	}
	if err := dbif.SubmitPullRequestReview(absId, "bob", model.PULL_REQUEST_REVIEW_APPROVED, tip); err != nil {
		t.Fatal(err)
	}
	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); !errors.Is(err, model.ErrPullRequestReviewRequired) {
		t.Fatalf("one of two reviewers approved, merge returned %v", err)
	}
	if err := dbif.SubmitPullRequestReview(absId, "carol", model.PULL_REQUEST_REVIEW_APPROVED, tip); err != nil {
		t.Fatal(err)
	}
	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); err != nil {
		t.Fatalf("merge with both approvals: %s", err)
	}
}

func TestPullRequestReviewChangesRequestedAndRemoval(t *testing.T) {
	dbif, _, forkPath, absId := setupMergeFixture(t, "changes and removal")
	tip := strings.TrimSpace(mergeTestGit(t, forkPath, "rev-parse", "master"))
	if err := dbif.AddPullRequestReviewer(absId, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := dbif.SubmitPullRequestReview(absId, "bob", model.PULL_REQUEST_REVIEW_CHANGES_REQUESTED, tip); err != nil {
		t.Fatal(err)
	}
	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); !errors.Is(err, model.ErrPullRequestReviewRequired) {
		t.Fatalf("merge with changes requested returned %v", err)
	}
	if err := dbif.RemovePullRequestReviewer(absId, "bob"); err != nil {
		t.Fatal(err)
	}
	reviews, err := dbif.GetPullRequestReviews(absId)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 0 {
		t.Fatalf("reviews after removal = %+v", reviews)
	}
	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); err != nil {
		t.Fatalf("merge after removing the reviewer: %s", err)
	}

	// only an assigned reviewer can submit a review.
	if err := dbif.SubmitPullRequestReview(absId, "nobody", model.PULL_REQUEST_REVIEW_APPROVED, tip); !errors.Is(err, db.ErrEntityNotFound) {
		t.Fatalf("review by an unassigned user returned %v", err)
	}
}

func TestPullRequestReviewInvalidatedByNewCommit(t *testing.T) {
	dbif, _, forkPath, absId := setupMergeFixture(t, "new commit")
	tip := strings.TrimSpace(mergeTestGit(t, forkPath, "rev-parse", "master"))
	if err := dbif.AddPullRequestReviewer(absId, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := dbif.SubmitPullRequestReview(absId, "bob", model.PULL_REQUEST_REVIEW_APPROVED, tip); err != nil {
		t.Fatal(err)
	}

	work := path.Join(t.TempDir(), "fork-more")
	mergeTestGit(t, t.TempDir(), "clone", "-q", forkPath, work)
	mergeTestGit(t, work, "config", "user.email", "b@example.com")
	mergeTestGit(t, work, "config", "user.name", "B")
	mergeTestWrite(t, work, "c.txt", "more")
	mergeTestGit(t, work, "add", "c.txt")
	mergeTestGit(t, work, "commit", "-q", "-m", "more")
	mergeTestGit(t, work, "push", "-q", "origin", "master")

	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); !errors.Is(err, model.ErrPullRequestReviewStale) {
		t.Fatalf("merge after a new provider commit returned %v", err)
	}
	newTip := strings.TrimSpace(mergeTestGit(t, forkPath, "rev-parse", "master"))
	if newTip == tip {
		t.Fatal("the test did not create a new commit")
	}
	if err := dbif.SubmitPullRequestReview(absId, "bob", model.PULL_REQUEST_REVIEW_APPROVED, newTip); err != nil {
		t.Fatal(err)
	}
	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); err != nil {
		t.Fatalf("merge after re-approving the new commit: %s", err)
	}
}

func TestPullRequestReviewTableMigration(t *testing.T) {
	dbif, _, _, absId := setupMergeFixture(t, "review table migration")
	if _, err := dbif.connection.Exec(`DROP TABLE gitus_pull_request_review`); err != nil {
		t.Fatal(err)
	}
	if err := dbif.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %s", err)
	}
	if err := dbif.AddPullRequestReviewer(absId, "bob"); err != nil {
		t.Fatalf("AddPullRequestReviewer after migration: %s", err)
	}
	reviews, err := dbif.GetPullRequestReviews(absId)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews after migration = %+v", reviews)
	}
}
