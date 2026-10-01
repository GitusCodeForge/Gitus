//go:build ignore
package templates

import "fmt"
import "time"

func(s time.Time) string {
	n, _ := s.Zone()
	return n
}

