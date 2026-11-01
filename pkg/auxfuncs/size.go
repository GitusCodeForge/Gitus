package auxfuncs

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// sizeUnits is ordered from the largest unit to the smallest. The multipliers
// are powers of 1024: the limit is reported to the user in the same units
// that `du` and a file manager show.
var sizeUnits = []struct {
	name string
	size int64
}{
	{"TiB", 1 << 40},
	{"GiB", 1 << 30},
	{"MiB", 1 << 20},
	{"KiB", 1 << 10},
}

// ParseSize reads a size a person typed into a configuration form, such as
// "512", "512M", "512 MiB" or "1.5 GiB", and returns the number of bytes.
// A bare number is bytes. An empty string and "0" both mean "no limit".
//
// The binary multipliers are used for every spelling (K, KB and KiB are all
// 1024 bytes), because the same number is shown back with FormatSize and the
// two have to round-trip.
func ParseSize(s string) (int64, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	if len(t) <= 0 || t == "0" || t == "unlimited" || t == "none" {
		return 0, nil
	}
	i := 0
	for i < len(t) {
		c := t[i]
		if ('0' <= c && c <= '9') || c == '.' {
			i++
			continue
		}
		break
	}
	number := t[:i]
	unit := strings.TrimSpace(t[i:])
	if len(number) <= 0 {
		return 0, fmt.Errorf("no number in %q", s)
	}
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot read the number in %q", s)
	}
	if value < 0 {
		return 0, errors.New("a size cannot be negative")
	}
	var multiplier int64 = 1
	switch unit {
	case "", "b", "byte", "bytes":
	case "k", "kb", "kib":
		multiplier = 1 << 10
	case "m", "mb", "mib":
		multiplier = 1 << 20
	case "g", "gb", "gib":
		multiplier = 1 << 30
	case "t", "tb", "tib":
		multiplier = 1 << 40
	default:
		return 0, fmt.Errorf("unknown size unit %q", unit)
	}
	return int64(value * float64(multiplier)), nil
}

// FormatSize renders a number of bytes for display, e.g. "1.5 GiB". Zero is
// rendered as "0 B"; callers that treat zero as "no limit" should say so
// themselves.
func FormatSize(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	for _, u := range sizeUnits {
		if n >= u.size {
			return fmt.Sprintf("%.1f %s", float64(n)/float64(u.size), u.name)
		}
	}
	return fmt.Sprintf("%d B", n)
}

// FormatSizeExact renders a number of bytes without rounding, so a value read
// from a configuration form and written back unchanged stays the same number
// of bytes. It uses the largest unit that divides the value exactly.
func FormatSizeExact(n int64) string {
	if n <= 0 {
		return "0"
	}
	for _, u := range sizeUnits {
		if n%u.size == 0 {
			return fmt.Sprintf("%d %s", n/u.size, u.name)
		}
	}
	return fmt.Sprintf("%d B", n)
}
