//go:build ignore
package templates

import ht "html/template"
import "github.com/gomarkdown/markdown"
import "github.com/microcosm-cc/bluemonday"

func(s string) ht.HTML {
	rs := string(markdown.ToHTML([]byte(s), nil, nil))
	rs = bluemonday.StrictPolicy().Sanitize(rs)
	return ht.HTML(rs)
}
