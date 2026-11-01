package session

import "testing"

func TestDescribeUserAgent(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want string
	}{
		{"firefox on linux", "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0", "Firefox on Linux"},
		{"chrome on windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36", "Chrome on Windows"},
		{"edge before chrome", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36 Edg/128.0", "Edge on Windows"},
		{"safari on macos", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari on macOS"},
		{"chrome on android", "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Mobile Safari/537.36", "Chrome on Android"},
		{"safari on ios", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1", "Safari on iOS"},
		{"netsurf", "Mozilla/5.0 (X11; Linux) NetSurf/3.11", "NetSurf on Linux"},
		{"curl", "curl/8.5.0", "curl"},
		{"lynx", "Lynx/2.9.0 libwww-FM/2.14", "Lynx"},
		{"only platform", "Mozilla/5.0 (X11; Linux x86_64)", "Linux"},
		{"empty", "", "unknown client"},
		{"unknown short", "SomethingElse/1.0", "SomethingElse/1.0"},
		{"unknown long", "something-very-long-that-does-not-look-like-any-known-client-at-all/1.0", "something-very-long-that-does-not-look-like-any-known-client..."},
	}
	for _, c := range cases {
		if got := DescribeUserAgent(c.ua); got != c.want {
			t.Errorf("%s: DescribeUserAgent = %q; want %q", c.name, got, c.want)
		}
	}
}
