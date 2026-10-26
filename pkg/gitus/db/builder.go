package db

import (
	"fmt"
	"strings"
)

// QueryBuilder assembles the WHERE conditions of a listing query together
// with the arguments those conditions need.
//
// Placeholders are obtained through Arg instead of being written by hand, so
// that values and placeholders always stay in the order the SQL text expects.
// The same builder renders either "?" (sqlite3) or numbered placeholders such
// as "$1" (postgres), which means condition fragments never have to be
// renumbered by hand when they are combined in different ways.
type QueryBuilder struct {
	numbered bool
	conds    []string
	args     []any
}

// NewQueryBuilder returns a builder that renders "?" placeholders.
func NewQueryBuilder() *QueryBuilder { return &QueryBuilder{} }

// NewNumberedQueryBuilder returns a builder that renders numbered
// placeholders ("$1", "$2", ...) instead of "?".
func NewNumberedQueryBuilder() *QueryBuilder { return &QueryBuilder{numbered: true} }

// Arg records v and returns the placeholder that refers to it.
func (b *QueryBuilder) Arg(v any) string {
	b.args = append(b.args, v)
	if b.numbered {
		return fmt.Sprintf("$%d", len(b.args))
	}
	return "?"
}

// And appends a condition. Empty conditions are ignored, so call sites do not
// have to track whether an "AND" is needed.
func (b *QueryBuilder) And(cond string) *QueryBuilder {
	if len(strings.TrimSpace(cond)) <= 0 { return b }
	b.conds = append(b.conds, cond)
	return b
}

// Where renders the WHERE clause, or the empty string when nothing has been
// added.
func (b *QueryBuilder) Where() string {
	if len(b.conds) <= 0 { return "" }
	return "WHERE " + strings.Join(b.conds, " AND ")
}

// Limit renders a LIMIT/OFFSET clause with its own placeholders.
func (b *QueryBuilder) Limit(pageSize int64, pageNum int64) string {
	return fmt.Sprintf("LIMIT %s OFFSET %s", b.Arg(pageSize), b.Arg(pageNum * pageSize))
}

// Args returns the arguments recorded so far, in placeholder order.
func (b *QueryBuilder) Args() []any {
	return b.args
}
