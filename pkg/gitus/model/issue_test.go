package model

import (
	"slices"
	"testing"
)

func TestIssueLabelListFormat(t *testing.T) {
	if got := FormatIssueLabelList(nil); got != "[]" {
		t.Errorf("nil list = %q", got)
	}
	if got := FormatIssueLabelList([]string{"bug", "ui"}); got != `["bug","ui"]` {
		t.Errorf("list = %q", got)
	}
	if got := ParseIssueLabelList(`["bug","ui"]`); !slices.Equal(got, []string{"bug", "ui"}) {
		t.Errorf("parse = %v", got)
	}
	for _, s := range []string{"", "   ", "not json", `{"a":1}`, "null"} {
		if got := ParseIssueLabelList(s); len(got) != 0 {
			t.Errorf("ParseIssueLabelList(%q) = %v; want empty", s, got)
		}
	}
}

func TestWithWithoutIssueLabel(t *testing.T) {
	base := []string{"bug"}
	added := WithIssueLabel(base, "ui")
	if !slices.Equal(added, []string{"bug", "ui"}) {
		t.Errorf("WithIssueLabel = %v", added)
	}
	// the original list must not be modified.
	if !slices.Equal(base, []string{"bug"}) {
		t.Errorf("WithIssueLabel modified its argument: %v", base)
	}
	if got := WithIssueLabel(added, "ui"); !slices.Equal(got, []string{"bug", "ui"}) {
		t.Errorf("duplicate label = %v", got)
	}
	if got := WithoutIssueLabel(added, "bug"); !slices.Equal(got, []string{"ui"}) {
		t.Errorf("WithoutIssueLabel = %v", got)
	}
	if got := WithoutIssueLabel(added, "nope"); !slices.Equal(got, []string{"bug", "ui"}) {
		t.Errorf("removing a missing label = %v", got)
	}
	if got := WithoutIssueLabel(nil, "x"); len(got) != 0 {
		t.Errorf("remove from nil = %v", got)
	}
}
