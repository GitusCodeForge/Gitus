package admin

import (
	"testing"
	"time"
)

func TestDatetimeLocalRoundTrip(t *testing.T) {
	const original = "2026-11-02T02:00:00+08:00"
	want, err := time.Parse(time.RFC3339, original)
	if err != nil { t.Fatal(err) }
	input := toDatetimeLocal(original)
	if input == "" { t.Fatal("toDatetimeLocal returned an empty value") }
	got, err := parseDatetimeLocal(input)
	if err != nil { t.Fatal(err) }
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil { t.Fatal(err) }
	if !parsed.Equal(want) {
		t.Fatalf("round trip = %s; want %s", parsed, want)
	}
}

func TestParseDatetimeLocal(t *testing.T) {
	if v, err := parseDatetimeLocal(""); err != nil || v != "" {
		t.Fatalf("empty = %q, %v", v, err)
	}
	if _, err := parseDatetimeLocal("not a time"); err == nil {
		t.Fatal("parseDatetimeLocal accepted garbage")
	}
	if v, err := parseDatetimeLocal("2026-11-02T02:00:00"); err != nil || v == "" {
		t.Fatalf("seconds form = %q, %v", v, err)
	}
}
