package controller

import (
	"net/http"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

// serveSVGSymbolPreview writes one symbol of an SVG sprite as a standalone
// SVG document. It returns false when the id is not defined, so the caller can
// answer 404.
func serveSVGSymbolPreview(w http.ResponseWriter, info *gitlib.SVGInfo, id string) bool {
	if info == nil {
		return false
	}
	data := info.SymbolPreview(id)
	if data == nil {
		return false
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Disposition", "inline")
	w.Write(data)
	return true
}
