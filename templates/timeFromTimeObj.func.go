//go:build ignore
package templates

import "time"

func(s time.Time) string {
	return s.Format(time.TimeOnly)
}

