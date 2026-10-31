package routes

import (
	"errors"
	"path/filepath"
)

// RRDocSubdirectory is the directory below the static asset directory in
// which the reading-required documents are stored.
const RRDocSubdirectory = "_rrdoc"

// ErrRRDocPathEscapes is reported for a document name that would resolve
// outside the document directory.
var ErrRRDocPathEscapes = errors.New("the document path is outside the document directory")

// RRDocDirectory returns the directory the reading-required documents live in.
func RRDocDirectory(staticAssetDirectory string) string {
	return filepath.Join(staticAssetDirectory, RRDocSubdirectory)
}

// ResolveRRDocPath resolves a document name to a file below
// <staticAssetDirectory>/_rrdoc. Names that escape that directory (an
// absolute path is kept below it by filepath.Join; ".." components are not)
// are rejected, so a caller never opens or writes a file elsewhere.
func ResolveRRDocPath(staticAssetDirectory string, name string) (string, error) {
	base := RRDocDirectory(staticAssetDirectory)
	p := filepath.Clean(filepath.Join(base, name))
	rel, err := filepath.Rel(base, p)
	if err != nil { return "", ErrRRDocPathEscapes }
	// filepath.IsLocal expresses exactly what we need (rel stays within the
	// directory); it is also the check CodeQL models as a path-injection
	// sanitizer, so the returned path stays clean for its analysis.
	if !filepath.IsLocal(rel) { return "", ErrRRDocPathEscapes }
	return filepath.Join(base, rel), nil
}
