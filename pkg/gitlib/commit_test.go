package gitlib

import (
	"slices"
	"testing"
)

func TestCommitMessageTagList(t *testing.T) {
	tags := []string{"!no-webhook", "ci-skip"}
	cases := []struct {
		name    string
		message string
		want    []string
	}{
		{"own line", "fix: part 1\n\nnot done yet.\n\n!no-webhook\n", []string{"!no-webhook"}},
		{"inline", "fix: part 1\n\n!no-webhook should not be here in a sentence\n", nil},
		{"case and whitespace", "fix\n\n   !No-WebHook   \n", []string{"!no-webhook"}},
		{"several tags", "fix\n\n!no-webhook\nci-skip\n", []string{"!no-webhook", "ci-skip"}},
		{"order", "fix\n\nci-skip\n!no-webhook\n", []string{"ci-skip", "!no-webhook"}},
		{"duplicates", "fix\n\n!no-webhook\n!no-webhook\n", []string{"!no-webhook"}},
		{"no tags", "fix: nothing to see here\n", nil},
		{"co-authored-by is not one of ours", "fix\n\nCo-Authored-By: A <a@example.com>\n", nil},
		{"empty message", "", nil},
	}
	for _, c := range cases {
		got := CommitMessageTagList(c.message, tags)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: CommitMessageTagList = %v; want %v", c.name, got, c.want)
		}
	}
	if got := CommitMessageTagList("fix\n\n!no-webhook\n", nil); got != nil {
		t.Errorf("no configured tags = %v; want nil", got)
	}
	if got := CommitMessageTagList("fix\n\n!no-webhook\n", []string{"", "  "}); got != nil {
		t.Errorf("only blank configured tags = %v; want nil", got)
	}
}
