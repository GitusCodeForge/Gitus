package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// rebind turns the PostgreSQL-style $1 placeholders used by the queries
// (which were written for the postgres backend and are shared with this port)
// into the ? placeholders MySQL expects. A placeholder may appear more than
// once, so the argument list is rebuilt from the indices rather than copied.
func rebind(query string, args []any) (string, []any) {
	if !strings.ContainsRune(query, '$') { return query, normalizeArgs(args) }
	var b strings.Builder
	b.Grow(len(query))
	newArgs := make([]any, 0, len(args))
	for i := 0; i < len(query); {
		if query[i] != '$' {
			b.WriteByte(query[i])
			i++
			continue
		}
		j := i + 1
		for j < len(query) && '0' <= query[j] && query[j] <= '9' { j++ }
		if j == i+1 {
			b.WriteByte('$')
			i++
			continue
		}
		n, err := strconv.Atoi(query[i+1:j])
		if err != nil || n < 1 || n > len(args) {
			// Leave the unexpandable placeholder alone so that the database
			// reports the query instead of a silently wrong argument list.
			b.WriteString(query[i:j])
			i = j
			continue
		}
		b.WriteByte('?')
		newArgs = append(newArgs, normalizeArg(args[n-1]))
		i = j
	}
	return b.String(), newArgs
}

func (dbif *MysqlGitusDatabaseInterface) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	q, args = rebind(q, args)
	return dbif.connection.QueryContext(ctx, q, args...)
}

func (dbif *MysqlGitusDatabaseInterface) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	q, args = rebind(q, args)
	return dbif.connection.QueryRowContext(ctx, q, args...)
}

func (dbif *MysqlGitusDatabaseInterface) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	q, args = rebind(q, args)
	return dbif.connection.ExecContext(ctx, q, args...)
}

func (dbif *MysqlGitusDatabaseInterface) begin(ctx context.Context) (*sql.Tx, error) {
	return dbif.connection.BeginTx(ctx, nil)
}

func queryTx(ctx context.Context, tx *sql.Tx, q string, args ...any) (*sql.Rows, error) {
	q, args = rebind(q, args)
	return tx.QueryContext(ctx, q, args...)
}

func queryRowTx(ctx context.Context, tx *sql.Tx, q string, args ...any) *sql.Row {
	q, args = rebind(q, args)
	return tx.QueryRowContext(ctx, q, args...)
}

func execTx(ctx context.Context, tx *sql.Tx, q string, args ...any) (sql.Result, error) {
	q, args = rebind(q, args)
	return tx.ExecContext(ctx, q, args...)
}

// normalizeArgs applies normalizeArg to every argument.
func normalizeArgs(args []any) []any {
	if len(args) == 0 { return args }
	res := make([]any, len(args))
	for i, a := range args { res[i] = normalizeArg(a) }
	return res
}

// normalizeArg makes an argument acceptable to database/sql. pgx encodes
// structs and maps as JSON on its own (the columns are JSON/JSONB); the MySQL
// driver only understands the basic driver.Value types, so those values are
// marshalled here. Pointers are dereferenced first so that new(string) keeps
// meaning the empty string.
func normalizeArg(a any) any {
	if a == nil { return nil }
	if _, ok := a.(driver.Valuer); ok { return a }
	rv := reflect.ValueOf(a)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() { return nil }
		rv = rv.Elem()
	}
	a = rv.Interface()
	switch a.(type) {
	case string, []byte, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, time.Time:
		return a
	}
	switch rv.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		b, err := json.Marshal(a)
		if err == nil { return string(b) }
	}
	return a
}

// nullTimeUnix returns the unix time of a nullable DATETIME, or 0 when the
// column is NULL.
func nullTimeUnix(t sql.NullTime) int64 {
	if !t.Valid { return 0 }
	return t.Time.Unix()
}
