//go:build ignore
package templates

import "strings"

func(s string, a string) bool {
	return strings.Contains(s, a)
}
