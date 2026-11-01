package auxfuncs

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"unlimited", 0},
		{"512", 512},
		{"512 B", 512},
		{"1K", 1024},
		{"1 KB", 1024},
		{"1 KiB", 1024},
		{"1.5 MiB", 3 << 19},
		{"2G", 2 << 30},
		{" 3 gib ", 3 << 30},
		{"1T", 1 << 40},
	}
	for _, c := range cases {
		got, err := ParseSize(c.in)
		if err != nil {
			t.Errorf("ParseSize(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) = %d; want %d", c.in, got, c.want)
		}
	}
	for _, in := range []string{"abc", "-1G", "1X", "1.2.3M"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) accepted an invalid size", in)
		}
	}
}

func TestFormatSizeRoundTrip(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1 << 20, "1.0 MiB"},
		{3 << 30, "3.0 GiB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q; want %q", c.in, got, c.want)
		}
	}
	// a formatted size must parse back to itself, so the form can show the
	// current value and accept it unchanged.
	for _, n := range []int64{1024, 1536, 3 << 30, 1 << 40} {
		got, err := ParseSize(FormatSize(n))
		if err != nil {
			t.Errorf("ParseSize(FormatSize(%d)): %v", n, err)
			continue
		}
		if got != n {
			t.Errorf("ParseSize(FormatSize(%d)) = %d", n, got)
		}
	}
}

func TestFormatSizeExact(t *testing.T) {
	cases := map[int64]string{
		0:       "0",
		512:     "512 B",
		1024:    "1 KiB",
		1536:    "1536 B",
		1 << 20: "1 MiB",
		3 << 30: "3 GiB",
	}
	for in, want := range cases {
		if got := FormatSizeExact(in); got != want {
			t.Errorf("FormatSizeExact(%d) = %q; want %q", in, got, want)
		}
		got, err := ParseSize(FormatSizeExact(in))
		if err != nil {
			t.Errorf("ParseSize(FormatSizeExact(%d)): %v", in, err)
			continue
		}
		if got != in {
			t.Errorf("ParseSize(FormatSizeExact(%d)) = %d", in, got)
		}
	}
}
