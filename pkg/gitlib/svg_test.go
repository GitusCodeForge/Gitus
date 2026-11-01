package gitlib

import (
	"encoding/xml"
	"strings"
	"testing"
)

const testSprite = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 48 48">
  <title>icons</title>
  <style>.red { fill: #c00; }</style>
  <defs>
    <symbol id="circle" viewBox="0 0 10 10">
      <circle cx="5" cy="5" r="4"/>
    </symbol>
  </defs>
  <symbol id="square" viewBox="0 0 12 12">
    <rect x="1" y="1" width="10" height="10" class="red"/>
  </symbol>
  <symbol id="empty"/>
</svg>`

func TestParseSVGSprite(t *testing.T) {
	info, err := ParseSVG([]byte(testSprite))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsSprite {
		t.Fatal("the document was not recognized as a sprite")
	}
	if len(info.Symbols) != 3 {
		t.Fatalf("found %d symbols: %+v", len(info.Symbols), info.Symbols)
	}
	if info.Symbols[0].ID != "circle" || info.Symbols[0].ViewBox != "0 0 10 10" {
		t.Errorf("first symbol = %+v", info.Symbols[0])
	}
	if info.Symbols[1].ID != "square" || info.Symbols[2].ID != "empty" {
		t.Errorf("symbols = %+v", info.Symbols)
	}
}

func TestSVGSymbolPreview(t *testing.T) {
	info, err := ParseSVG([]byte(testSprite))
	if err != nil {
		t.Fatal(err)
	}
	preview := info.SymbolPreview("square")
	if len(preview) <= 0 {
		t.Fatal("no preview for square")
	}
	s := string(preview)
	for _, want := range []string{
		`viewBox="0 0 12 12"`,
		`<use href="#square"`,
		`xlink:href="#square"`,
		`class="red"`, // the original document content is kept
	} {
		if !strings.Contains(s, want) {
			t.Errorf("preview does not contain %q:\n%s", want, s)
		}
	}
	openTag := s[:strings.Index(s, ">")+1]
	if strings.Count(openTag, "viewBox=") != 1 {
		t.Errorf("the root element should carry exactly one viewBox: %s", openTag)
	}
	// A preview has to be well-formed XML, or the browser will not draw it.
	var holder struct{}
	if err := xml.Unmarshal(preview, &holder); err != nil {
		t.Fatalf("preview is not well-formed: %s\n%s", err, s)
	}
	if info.SymbolPreview("does-not-exist") != nil {
		t.Error("a preview was built for an unknown symbol")
	}
}

func TestParseSVGNotASprite(t *testing.T) {
	info, err := ParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10z"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if info.IsSprite {
		t.Error("an SVG with visible content was treated as a sprite")
	}
	if len(info.Symbols) != 0 {
		t.Errorf("symbols = %+v", info.Symbols)
	}
}

func TestParseSVGRejects(t *testing.T) {
	if _, err := ParseSVG([]byte(`<html></html>`)); err != ErrNotSVG {
		t.Errorf("non-svg root returned %v", err)
	}
	if _, err := ParseSVG([]byte(`<svg><symbol id="x">`)); err == nil {
		t.Error("malformed XML was accepted")
	}
	big := make([]byte, MaxSVGSymbolBytes+1)
	if _, err := ParseSVG(big); err != ErrSVGTooLarge {
		t.Errorf("oversized document returned %v", err)
	}
}

func TestIsSVG(t *testing.T) {
	yes := [][]byte{
		[]byte(`<svg></svg>`),
		[]byte("  \n<svg viewBox=\"0 0 1 1\"></svg>"),
		[]byte("\xEF\xBB\xBF<svg></svg>"),
		[]byte(`<?xml version="1.0"?><svg></svg>`),
		[]byte("<!-- comment --><svg></svg>"),
		[]byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg></svg>`),
	}
	for _, b := range yes {
		if !IsSVG(b) {
			t.Errorf("IsSVG(%q) = false", string(b))
		}
	}
	no := [][]byte{
		nil,
		[]byte("<?xml version=\"1.0\"?><html></html>"),
		[]byte("hello <svg>"),
		[]byte(`<div><svg></svg></div>`),
	}
	for _, b := range no {
		if IsSVG(b) {
			t.Errorf("IsSVG(%q) = true", string(b))
		}
	}
}

func TestParseSVGPrefixedRoot(t *testing.T) {
	data := []byte(`<svg:svg xmlns:svg="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><svg:symbol id="dot" viewBox="0 0 8 8"><svg:circle cx="4" cy="4" r="3"/></svg:symbol></svg:svg>`)
	info, err := ParseSVG(data)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsSprite || len(info.Symbols) != 1 || info.Symbols[0].ID != "dot" {
		t.Fatalf("symbols = %+v", info.Symbols)
	}
	preview := info.SymbolPreview("dot")
	if len(preview) <= 0 {
		t.Fatal("no preview")
	}
	for _, want := range []string{`<use href="#dot"`, `</svg:svg>`} {
		if !strings.Contains(string(preview), want) {
			t.Errorf("preview does not contain %q:\n%s", want, string(preview))
		}
	}
	var holder struct{}
	if err := xml.Unmarshal(preview, &holder); err != nil {
		t.Fatalf("preview is not well-formed: %s\n%s", err, string(preview))
	}
}
