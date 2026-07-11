package controller

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/alecthomas/chroma/v2"
	chromaHtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"golang.org/x/crypto/bcrypt"
)

func basicStringEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "\"", "\\\"")
}

func writeTree(repo *gitlib.LocalGitRepository, w *zip.Writer, pathPrefix string, tree *gitlib.TreeObject) error {
	// `pathPrefix` should be empty or end with slash.
	for _, item := range tree.ObjectList {
		pathname := fmt.Sprintf("%s%s", pathPrefix, item.Name)
		obj, err := repo.ReadObject(item.Hash)
		if err != nil { return err }
		switch item.Mode {
		case gitlib.TREE_NORMAL_FILE: fallthrough
		case gitlib.TREE_EXECUTABLE_FILE: fallthrough
		case gitlib.TREE_SYMBOLIC_LINK:
			// go's zip library in stdlib seems to not have anything
			// that supports ymbolic links. we might get away with not
			// supporting it...
			wr, err := w.Create(pathname)
			if err != nil { return err }
			if obj.Type() != gitlib.BLOB {
				return fmt.Errorf("%s is not a blob object", obj.ObjectId())
			}
			wr.Write(obj.RawData())
		case gitlib.TREE_TREE_OBJECT:
			tobj, ok := obj.(*gitlib.TreeObject)
			if !ok {
				return fmt.Errorf("%s is not a blob object", obj.ObjectId())
			}
			writeTree(repo, w, pathname+"/", tobj)
		case gitlib.TREE_SUBMODULE:
			// we don't support submodule at the moment...
		}
	}
	return nil
}

func responseWithTreeZip(repo *gitlib.LocalGitRepository, obj gitlib.GitObject, name string, w http.ResponseWriter) error {
	// requires:
	// + `name` to be descriptive and without the `.zip` extension name.
	// + `obj` to be a tree object.
	tobj, ok := obj.(*gitlib.TreeObject)
	if !ok {
		return fmt.Errorf(
			"%s is not a tree object",
			obj.ObjectId(),
		)
	}
	filenameStar := url.QueryEscape(fmt.Sprintf("%s.zip", name))
	// it was said that "browsers handle escape sequences
	// differently", but i would assume that most of them would at
	// least handle \" and \\...
	filename := fmt.Sprintf("\"%s.zip\"", basicStringEscape(name))
	w.Header().Add(
		"Content-Disposition",
		fmt.Sprintf("attachment; filename=%s; filename*=UTF-8''%s", filename, filenameStar),
	)
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()
	err := writeTree(repo, zipWriter, "", tobj)
	if err != nil { return err }
	zipWriter.Flush()
	return nil
}


// NOTE: Chroma's `Analyse` is basically useless if we don't discern
// language type manually; it somehow can reach to the conclusion that
// C, Nim and GAS Assembly are all GDScript 3. i'm tempted to make
// my own syntax coloring engine here.
func codeTypeDiscern(s string) string {
	switch s {
	case ".as": return "ActionScript"
	case ".antlr4": return "ANTLR"
	case ".ads": fallthrough
	case ".adb": return "Ada"
	case ".awk": return "Awk"
	case ".agda": return "Agda"
	case ".sh": return "Bash"
	case ".bibtex": return "BibTeX"
	case ".bat": return "Batchfile"
	case ".bf": return "Brainfuck"
	case ".c": return "C"
	case ".h": return "C"
	case ".cpp": return "C++"
	case ".hpp": return "C++"
	case ".cs": return "C#"
	case ".cbl": return "COBOL"
	case ".crystal": return "Crystal"
	case ".lisp": return "Common Lisp"
	case ".clj": fallthrough
	case ".cljs": return "Clojure"
	case ".css": return "CSS"
	case ".d": return "D"
	case ".dart": return "Dart"
	case ".dtd": return "DTD"
	case ".elm": return "Elm"
	case ".el": return "EmacsLisp"
	case ".erl": return "Erlang"
	case ".ex": fallthrough
	case ".exs": return "Elixir"
	case ".for": return "FortranFixed"
	case ".f95": return "Fortran"
	case ".fs": return "FSharp"
	case ".fish": return "Fish"
	case ".fth": fallthrough
	case ".forth": return "Forth"
	case ".factor": return "Factor"
	case ".S": return "GAS"
	case ".desktop": return "Desktop Entry"
	case ".jinja": return "Django/Jinja"
	case ".dylan": return "Dylan"
	case ".diff": return "Diff"
	case ".gleam": return "Gleam"
	case ".groovy": return "Groovy"
	case ".glsl": return "GLSL"
	case ".go": return "Go"
	case ".graphql": return "GraphQL"
	case ".nim": return "Nim"
	case ".md": return "Markdown"
	case ".ws": return "APL"
	case ".hs": return "Haskell"
	case ".haxe": return "Haxe"
	case ".HC": return "HolyC"
	case ".shtml": fallthrough
	case ".xhtml": fallthrough
	case ".dhtml": fallthrough
	case ".htm": fallthrough
	case ".html": return "HTML"
	case ".hy": return "Hy"
	case ".idris": return "Idris"
	case ".ini": return "INI"
	case ".io": return "Io"
	case ".j": return "J"
	case ".java": return "Java"
	case ".mjs": fallthrough
	case ".js": return "JavaScript"
	case ".json": return "JSON"
	case ".julia": return "Julia"
	case ".kt": return "Kotlin"
	case ".lean": return "Lean"
	case ".lua": return "Lua"
	case ".nix": return "Nix"
	case ".ml": return "OCaml"
	case ".odin": return "Odin"
	case ".org": return "Org Mode"
	case ".php": return "PHP"
	case ".ps1": return "PowerShell"
	case ".py": return "Python"
	case ".r": return "R"
	case ".rkt": return "Racket"
	case ".rst": return "reStructuredText"
	case ".rexx": return "Rexx"
	case ".rb": return "Ruby"
	case ".rs": return "Rust"
	case ".scala": return "Scala"
	case ".scm": return "Scheme"
	case ".scss": return "SCSS"
	case ".solidity": return "Solidity"
	case ".sql": return "SQL"
	case ".sml": return "Standard ML"
	case ".svelte": return "Svelte"
	case ".swift": return "Swift"
	case ".tcl": return "Tcl"
	case ".tex": return "TeX"
	case ".toml": return "TOML"
	case ".ts": return "TypeScript"
	case ".txt": return "plaintext"
	case ".typst": return "Typst"
	case ".vala": return "Vala"
	case ".vue": return "Vue"
	case ".xml": return "XML"
	case ".yml": fallthrough
	case ".yaml": return "YAML"
	case ".zig": return "Zig"
	case ".asm": return "NASM"
	default: return ""
	}
}

func colorSyntax(filename string, s string) (string, error) {
	var lexer chroma.Lexer
	switch filename {
	case "Dockerfile": lexer = lexers.Get("Docker")
	case "Makefile": lexer = lexers.Get("Makefile")
	case "": lexer = lexers.Analyse(s)
	default:
		codeType := codeTypeDiscern(path.Ext(filename))
		if codeType == "" {
			lexer = lexers.Analyse(s)
		} else {
			lexer = lexers.Get(codeType)
		}
	}
	if lexer == nil {
		return html.EscapeString(s), nil
	}
	style := styles.Get("algol")
	if style == nil { style = styles.Fallback }
	formatter := chromaHtml.New(chromaHtml.PreventSurroundingPre(true))
	iterator, err := lexer.Tokenise(nil, s)
	if err != nil { return "", err }
	buf := new(bytes.Buffer)
	err = formatter.Format(buf, style, iterator)
	if err != nil { return "", err }
	return buf.String(), nil
}

func checkUserPassword(ctx *routes.RouterContext, username string, password string) (bool, error) {
	user, err := ctx.DatabaseInterface.GetUserByName(username)
	if err != nil { return false, err }
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword { return false, nil }
		return false, err
	}
	return true, nil
}

func newConfirmCode() string {
	res := make([]byte, 0)
	rmax := big.NewInt(8)
	for range 6 {
		n, _ := rand.Int(rand.Reader, rmax)
		res = append(res, "0123456789ABCDEFGHJKLMNPQRSTUVWXYZ"[n.Uint64()])
	}
	return string(res)
}

func getQueryPath(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil { return "", err }
	return fmt.Sprintf("/%s?%s",
		url.QueryEscape(u.Path[1:]),
		url.QueryEscape(u.Query().Encode())), nil
}

