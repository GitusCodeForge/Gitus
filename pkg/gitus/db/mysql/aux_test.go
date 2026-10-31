package mysql

import "testing"

func TestRebind(t *testing.T) {
	type tuple struct{ A string `json:"a"` }

	cases := []struct {
		name string
		in   string
		args []any
		want string
		wantArgs []any
	}{
		{"none", "SELECT 1", nil, "SELECT 1", nil},
		{"simple", "SELECT * FROM t WHERE a = $1", []any{"x"}, "SELECT * FROM t WHERE a = ?", []any{"x"}},
		{"reuse", "SELECT $1, $1, $2", []any{"a", "b"}, "SELECT ?, ?, ?", []any{"a", "a", "b"}},
		{"out of order", "UPDATE t SET a = $2 WHERE b = $1", []any{"b", "a"}, "UPDATE t SET a = ? WHERE b = ?", []any{"a", "b"}},
		{"literal dollar", "SELECT '$'", nil, "SELECT '$'", nil},
		{"json arg", "SELECT $1", []any{tuple{A: "v"}}, "SELECT ?", []any{`{"a":"v"}`}},
		{"pointer arg", "SELECT $1", []any{new(string)}, "SELECT ?", []any{""}},
	}
	for _, c := range cases {
		got, gotArgs := rebind(c.in, c.args)
		if got != c.want {
			t.Errorf("%s: query = %q, want %q", c.name, got, c.want)
			continue
		}
		if len(gotArgs) != len(c.wantArgs) {
			t.Errorf("%s: %d args, want %d", c.name, len(gotArgs), len(c.wantArgs))
			continue
		}
		for i := range gotArgs {
			if gotArgs[i] != c.wantArgs[i] {
				t.Errorf("%s: arg %d = %#v, want %#v", c.name, i, gotArgs[i], c.wantArgs[i])
			}
		}
	}
}
