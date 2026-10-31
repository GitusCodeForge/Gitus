//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitlib"

type TagInfoTemplateModel struct {
	// it should be made sure that when Annotated is true, Tag is nil,
	// and vice versa.
	Annotated bool
	RepoName string
	Namespace string
	Name string
	Tag *gitlib.TagObject
	EmailUserMapping map[string]string
}

