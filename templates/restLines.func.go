//go:build ignore
package templates

import "strings"

func(s string) []string {
	a := strings.SplitN(strings.TrimSpace(s), "\n", 2)
	alen := len(a)
	if alen < 2 { return nil }
	return a[1:]
}

