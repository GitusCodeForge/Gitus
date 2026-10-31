package controller

import (
	"github.com/alecthomas/chroma/v2"
	chromaHtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/microcosm-cc/bluemonday"
)

// The sanitizer policies and the Chroma style/formatter are stateless once
// built, so they are built once here rather than on every rendered page.
var (
	// strictSanitizer is used for repository readmes. The markdown and org
	// renderers emit raw HTML, so only a conservative whitelist may survive.
	strictSanitizer = bluemonday.StrictPolicy()
	// ugcSanitizer is used for the other user-authored pages (front page,
	// rrdoc); it allows a reasonable subset of HTML.
	ugcSanitizer = bluemonday.UGCPolicy()
)

var (
	chromaStyle = func() *chroma.Style {
		if s := styles.Get("algol"); s != nil { return s }
		return styles.Fallback
	}()
	chromaFormatter = chromaHtml.New(chromaHtml.PreventSurroundingPre(true))
)
