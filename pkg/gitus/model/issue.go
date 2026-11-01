package model

import (
	"encoding/json"
	"slices"
	"strings"
)

type Issue struct {
	IssueAbsId int64
	RepoNamespace string
	RepoName string
	IssueId int
	IssueAuthor string
	IssueTitle string
	IssueContent string
	IssueTime int64
	IssueStatus int
	IssuePriority int
	// LabelList is the subset of the repository labels attached to this
	// issue. The vocabulary is the repository's label list; see
	// Repository.RepoLabelList.
	LabelList []string
}

// ParseIssueLabelList decodes the JSON array stored in the issue_label column.
// A malformed value yields an empty list: labels are decoration, and a bad
// value should not make the issue unreadable.
func ParseIssueLabelList(s string) []string {
	if len(strings.TrimSpace(s)) <= 0 { return nil }
	res := make([]string, 0)
	if err := json.Unmarshal([]byte(s), &res); err != nil { return nil }
	return res
}

// FormatIssueLabelList encodes a label list for the issue_label column.
func FormatIssueLabelList(l []string) string {
	if l == nil { l = make([]string, 0) }
	b, err := json.Marshal(l)
	if err != nil { return "[]" }
	return string(b)
}

// WithIssueLabel returns the list with label appended, without duplicates.
func WithIssueLabel(list []string, label string) []string {
	if slices.Contains(list, label) { return list }
	res := make([]string, len(list), len(list)+1)
	copy(res, list)
	return append(res, label)
}

// WithoutIssueLabel returns the list without label.
func WithoutIssueLabel(list []string, label string) []string {
	res := make([]string, 0, len(list))
	for _, l := range list {
		if l == label { continue }
		res = append(res, l)
	}
	return res
}

const (
	ISSUE_OPENED = 1
	ISSUE_CLOSED_AS_SOLVED = 2
	ISSUE_CLOSED_AS_DISCARDED = 3
)

const (
	EVENT_COMMENT = 1
	EVENT_CLOSED_AS_SOLVED = 2
	EVENT_CLOSED_AS_DISCARDED = 3
	EVENT_REOPENED = 4
)

type IssueEvent struct {
	EventAbsId int64
	EventType int
	EventTimestamp int64
	EventAuthor string
	EventContent string	
}

