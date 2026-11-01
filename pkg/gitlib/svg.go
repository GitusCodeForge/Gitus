package gitlib

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// MaxSVGSymbolBytes is the largest SVG document whose symbols are listed on
// the file page. An icon sprite is a few kilobytes; parsing something much
// larger on every view would only cost memory, and each symbol preview embeds
// the whole document.
const MaxSVGSymbolBytes = 1 << 20

// ErrNotSVG is returned by ParseSVG when the document's root element is not
// an SVG element.
var ErrNotSVG = errors.New("not an svg document")

// ErrSVGTooLarge is returned by ParseSVG for a document larger than
// MaxSVGSymbolBytes.
var ErrSVGTooLarge = errors.New("the svg document is too large to list its symbols")

// SVGSymbolInfo is one <symbol> definition of an SVG sprite.
type SVGSymbolInfo struct {
	ID      string
	ViewBox string
}

// SVGInfo is what ParseSVG learned about an SVG document. The unexported
// fields keep the raw ranges that SymbolPreview needs, so the document is not
// re-parsed for every preview.
type SVGInfo struct {
	// Symbols lists the <symbol> elements that have an id. It is only meant
	// to be shown when IsSprite is true.
	Symbols []SVGSymbolInfo
	// IsSprite is true when the document consists of definitions only (an
	// icon sprite) and has at least one symbol. Such a document renders
	// nothing on its own, so the file page lists the symbols instead.
	IsSprite bool

	viewBox       string
	rootStartTag  string
	inner         string
	symbolInner   map[string]string
	symbolViewBox map[string]string
}

var svgViewBoxAttrRE = regexp.MustCompile(`(?i)\s+viewBox\s*=\s*("[^"]*"|'[^']*')`)
var svgXMLNSAttrRE = regexp.MustCompile(`(?i)\sxmlns\s*=`)

// IsSVG reports whether data starts with an SVG root element. It only looks at
// the beginning of the file, so it is cheap even for a large blob; ParseSVG
// does the real validation.
func IsSVG(data []byte) bool {
	s := data
	if bytes.HasPrefix(s, []byte{0xEF, 0xBB, 0xBF}) {
		s = s[3:]
	}
	s = bytes.TrimLeft(s, " \t\r\n")
	for len(s) > 0 {
		switch {
		case bytes.HasPrefix(s, []byte("<?xml")):
			i := bytes.Index(s, []byte("?>"))
			if i < 0 {
				return false
			}
			s = s[i+2:]
		case bytes.HasPrefix(s, []byte("<!--")):
			i := bytes.Index(s, []byte("-->"))
			if i < 0 {
				return false
			}
			s = s[i+3:]
		case bytes.HasPrefix(s, []byte("<!DOCTYPE")):
			i := bytes.IndexByte(s, '>')
			if i < 0 {
				return false
			}
			s = s[i+1:]
		default:
			if len(s) < 4 || !bytes.EqualFold(s[:4], []byte("<svg")) {
				return false
			}
			return len(s) == 4 || bytes.IndexByte([]byte(" \t\r\n/>:"), s[4]) >= 0
		}
		s = bytes.TrimLeft(s, " \t\r\n")
	}
	return false
}

// ParseSVG reads the symbol definitions of an SVG document. It accepts the
// symbols wherever they are defined, including inside <defs>.
func ParseSVG(data []byte) (*SVGInfo, error) {
	if len(data) > MaxSVGSymbolBytes {
		return nil, ErrSVGTooLarge
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	info := &SVGInfo{
		symbolInner:   make(map[string]string),
		symbolViewBox: make(map[string]string),
	}
	depth := 0
	seenRoot := false
	var rootInnerStart int64
	spriteOnly := true
	inSymbol := false
	symbolDepth := 0
	var symbolID, symbolViewBox, symbolQualName string
	var symbolStartTagEnd int64
	var symbolSelfClosing bool

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !seenRoot {
				if !strings.EqualFold(t.Name.Local, "svg") {
					return nil, ErrNotSVG
				}
				seenRoot = true
				depth = 1
				end := dec.InputOffset()
				start := bytes.LastIndexByte(data[:end], '<')
				if start < 0 {
					return nil, ErrNotSVG
				}
				info.rootStartTag = strings.TrimSpace(string(data[start:end]))
				info.viewBox = attrValue(t.Attr, "viewBox")
				rootInnerStart = end
				continue
			}
			if depth == 1 {
				switch t.Name.Local {
				case "symbol", "defs", "style", "title", "desc", "metadata":
				default:
					// something that would be drawn on its own: not a pure
					// sprite, so the normal image view is the right one.
					spriteOnly = false
				}
			}
			if t.Name.Local == "symbol" && !inSymbol {
				inSymbol = true
				symbolDepth = depth
				symbolID = attrValue(t.Attr, "id")
				symbolViewBox = attrValue(t.Attr, "viewBox")
				symbolStartTagEnd = dec.InputOffset()
				symbolSelfClosing = symbolStartTagEnd >= 2 &&
					data[symbolStartTagEnd-2] == '/' && data[symbolStartTagEnd-1] == '>'
				start := bytes.LastIndexByte(data[:symbolStartTagEnd], '<')
				if start >= 0 {
					symbolQualName = elementName(string(data[start:symbolStartTagEnd]))
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if t.Name.Local == "symbol" && inSymbol && depth == symbolDepth {
				if len(symbolID) > 0 {
					if _, exists := info.symbolInner[symbolID]; !exists {
						info.symbolInner[symbolID] = symbolInner(data, symbolStartTagEnd, dec.InputOffset(), symbolSelfClosing, symbolQualName)
						info.symbolViewBox[symbolID] = symbolViewBox
						info.Symbols = append(info.Symbols, SVGSymbolInfo{ID: symbolID, ViewBox: symbolViewBox})
					}
				}
				inSymbol = false
			}
			if depth == 0 && seenRoot {
				end := dec.InputOffset()
				endTag := bytes.LastIndex(data[:end], []byte("</svg"))
				if endTag >= 0 && rootInnerStart <= int64(endTag) {
					info.inner = string(data[rootInnerStart:int64(endTag)])
				}
			}
		}
	}
	if !seenRoot {
		return nil, ErrNotSVG
	}
	info.IsSprite = spriteOnly && len(info.Symbols) > 0
	return info, nil
}

// symbolInner returns the content between a symbol's start and end tags. With
// the ranges from the decoder this is raw text, so the preview keeps whatever
// namespaces and attributes the original used.
func symbolInner(data []byte, startTagEnd int64, endOffset int64, selfClosing bool, qualName string) string {
	if selfClosing || len(qualName) <= 0 {
		return ""
	}
	endTag := bytes.LastIndex(data[:endOffset], []byte("</"+qualName))
	if endTag < 0 || int64(endTag) < startTagEnd {
		return ""
	}
	return string(data[startTagEnd:int64(endTag)])
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// SymbolPreview returns a standalone SVG document that draws one symbol, or
// nil when the id is unknown. The preview embeds the original document content
// so that <style> blocks, <defs> and references between symbols keep working,
// then renders the symbol once with <use>.
func (info *SVGInfo) SymbolPreview(id string) []byte {
	if _, ok := info.symbolInner[id]; !ok {
		return nil
	}
	open := strings.TrimSpace(info.rootStartTag)
	if strings.HasSuffix(open, "/>") {
		open = strings.TrimSuffix(open, "/>") + ">"
	}
	if !strings.HasSuffix(open, ">") {
		return nil
	}
	// Keep the element name (normally "svg", but it can be a prefixed one
	// such as "svg:svg") so the closing tag matches.
	rootName := elementName(open)
	if len(rootName) <= 0 {
		return nil
	}
	open = svgViewBoxAttrRE.ReplaceAllString(open, "")
	if !svgXMLNSAttrRE.MatchString(open) {
		open = open[:len(open)-1] + ` xmlns="http://www.w3.org/2000/svg">`
	}
	if !strings.Contains(open, "xmlns:xlink") {
		open = open[:len(open)-1] + ` xmlns:xlink="http://www.w3.org/1999/xlink">`
	}
	viewBox := info.symbolViewBox[id]
	if len(viewBox) <= 0 {
		viewBox = info.viewBox
	}
	if len(viewBox) > 0 {
		open = open[:len(open)-1] + fmt.Sprintf(` viewBox="%s">`, xmlEscapeAttr(viewBox))
	}
	escapedID := xmlEscapeAttr(id)
	use := fmt.Sprintf(`<use href="#%s" xlink:href="#%s" width="100%%" height="100%%"/>`, escapedID, escapedID)
	return []byte(open + info.inner + use + "</" + rootName + ">")
}

func xmlEscapeAttr(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// elementName returns the possibly prefixed name of a raw start tag, e.g.
// "svg" for "<svg ...>" and "svg:svg" for "<svg:svg ...>".
func elementName(tag string) string {
	if len(tag) < 2 || tag[0] != '<' {
		return ""
	}
	end := strings.IndexAny(tag[1:], " \t\r\n/>")
	if end < 0 {
		return ""
	}
	return tag[1 : 1+end]
}
