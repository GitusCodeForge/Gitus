//go:build ignore
package templates

import "strings"

func(s string) string {
	s = strings.TrimPrefix(s, "a/")
	s = strings.TrimPrefix(s, "b/")
	return s
}
