// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
)

// The renderer fails on a currentColor paint and ignores element and id
// selectors in <style>. resolveSVGPaint rewrites both into forms it reads, so
// the mark draws as a browser draws it.

const svgDefaultColor = "#000000"

// svgMaxPaintBytes bounds one substituted value: it is copied into every use,
// so its length multiplies into the rewritten document.
const svgMaxPaintBytes = 64

// svgPaintSlack is the growth allowed on top of twice the source.
const svgPaintSlack = 64 << 10

var (
	// svgCurrentDecl matches a CSS declaration whose whole value is currentColor,
	// so a selector, identifier or url(#currentColor) is never touched.
	svgCurrentDecl = regexp.MustCompile(`(?i)(:\s*)currentcolor(\s*(?:!\s*important)?\s*)([;}]|$)`)
	svgImportant   = regexp.MustCompile(`(?i)\s*!\s*important\s*$`)
	// svgSafePaint admits colour words, hex, rgb(), hsl() and url(#id).
	// Any other url() is refused, so the rewrite names no external resource.
	svgSafePaint   = regexp.MustCompile(`^(?:[#A-Za-z0-9 .,%+-]+|(?:rgba?|hsla?)\([0-9 .,%+-]+\)|url\(#[A-Za-z0-9_.:-]+\))$`)
	svgCSSComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	svgCurrentAttr = map[string]bool{"fill": true, "stroke": true, "stop-color": true, "flood-color": true, "lighting-color": true}
)

// svgCascadedProps are the properties a selector rule is inlined for.
var svgCascadedProps = []string{"fill", "stroke", "stop-color", "flood-color", "lighting-color"}

// svgAttrOnlyProps are read by the renderer from attributes alone, never from
// style, so a selector's value for them is written as an attribute.
var svgAttrOnlyProps = map[string]bool{"stop-color": true}

// svgCommentMark replaces a CSS comment. It fails svgSafePaint, so a value
// that is only valid once a comment is removed (u/**/rl) is dropped.
const svgCommentMark = "\x00"

// svgDecl is one CSS declaration value; important marks a !important one.
type svgDecl struct {
	value     string
	important bool
}

// svgSelectorPaint holds the winning fill and stroke per tag name and per id.
type svgSelectorPaint struct {
	byTag, byID map[string]map[string]svgDecl
}

type svgPaintScan struct {
	color string
	css   strings.Builder
}

func isCurrentColor(value string) bool {
	return strings.EqualFold(strings.TrimSpace(svgImportant.ReplaceAllString(value, "")), "currentcolor")
}

func svgPaintAllowed(value string) bool {
	return len(value) <= svgMaxPaintBytes && svgSafePaint.MatchString(value)
}

// resolveSVGPaint returns src with currentColor replaced by the root's colour.
// It also moves fill and stroke from tag and id selectors into the matching
// elements' style. A document it cannot tokenize comes back unchanged, and one
// that would grow past the size bound is refused.
func resolveSVGPaint(src []byte) ([]byte, error) {
	scan, ok := scanSVGPaint(src)
	if !ok {
		return src, nil
	}
	bound := min(2*len(src)+svgPaintSlack, MaxMarkBytes)
	rules := parseSVGSelectors(scan.css.String())
	decoder := xml.NewDecoder(bytes.NewReader(src))
	decoder.Strict = false
	var out bytes.Buffer
	copied, styleDepth := 0, 0
	for {
		start := int(decoder.InputOffset())
		token, err := decoder.RawToken()
		if err != nil {
			break
		}
		end := int(decoder.InputOffset())
		var replacement []byte
		switch t := token.(type) {
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "style") {
				styleDepth++
			}
			replacement = paintedStartTag(src[start:end], t, rules, scan.color)
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "style") {
				styleDepth--
			}
		case xml.CharData:
			if styleDepth > 0 && svgCurrentDecl.Match(t) {
				replacement = svgCurrentDecl.ReplaceAll(src[start:end], []byte("${1}"+scan.color+"${2}${3}"))
			}
		}
		if replacement != nil {
			out.Write(src[copied:start])
			out.Write(replacement)
			copied = end
			if out.Len() > bound {
				return nil, fmt.Errorf("%w: resolving the SVG's paint would grow it past %d bytes", ErrUnsupported, bound)
			}
		}
	}
	out.Write(src[copied:])
	if out.Len() > bound {
		return nil, fmt.Errorf("%w: resolving the SVG's paint would grow it past %d bytes", ErrUnsupported, bound)
	}
	return out.Bytes(), nil
}

func scanSVGPaint(src []byte) (*svgPaintScan, bool) {
	scan := &svgPaintScan{color: svgDefaultColor}
	decoder := xml.NewDecoder(bytes.NewReader(src))
	decoder.Strict = false
	styleDepth, rootSeen := 0, false
	for {
		token, err := decoder.RawToken()
		if err != nil {
			return scan, true
		}
		switch t := token.(type) {
		case xml.StartElement:
			if !rootSeen {
				rootSeen = true
				scan.color = svgRootColor(t.Attr)
			}
			if strings.EqualFold(t.Name.Local, "style") {
				styleDepth++
			}
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "style") {
				styleDepth--
			}
		case xml.CharData:
			if styleDepth > 0 {
				scan.css.Write(t)
				scan.css.WriteByte('\n')
			}
		}
	}
}

// svgRootColor reads the root's color attribute, or its color in style, which
// wins as it does in CSS.
func svgRootColor(attrs []xml.Attr) string {
	color := ""
	for _, attr := range attrs {
		switch strings.ToLower(attr.Name.Local) {
		case "color":
			color = strings.TrimSpace(attr.Value)
		case "style":
			if decl, ok := parseSVGDeclarations(attr.Value)["color"]; ok {
				color = decl.value
			}
		}
	}
	if color == "" || isCurrentColor(color) || !svgPaintAllowed(color) || strings.EqualFold(color, "inherit") {
		return svgDefaultColor
	}
	return color
}

// parseSVGDeclarations reads "a:b;c:d" into a map; the last declaration of a
// property wins.
func parseSVGDeclarations(block string) map[string]svgDecl {
	decls := map[string]svgDecl{}
	for pair := range strings.SplitSeq(block, ";") {
		key, value, found := strings.Cut(pair, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		important := svgImportant.MatchString(value)
		value = strings.TrimSpace(svgImportant.ReplaceAllString(value, ""))
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" && value != "" {
			decls[key] = svgDecl{value, important}
		}
	}
	return decls
}

// mergeDecl lets src override dst unless dst is !important and src is not.
func mergeDecl(dst map[string]svgDecl, src map[string]svgDecl) {
	for key, decl := range src {
		if old, ok := dst[key]; ok && old.important && !decl.important {
			continue
		}
		dst[key] = decl
	}
}

// parseSVGSelectors keeps the cascaded paint of rules whose selector is a bare
// tag name or #id. Class rules stay with the renderer, which applies them last.
func parseSVGSelectors(css string) svgSelectorPaint {
	rules := svgSelectorPaint{byTag: map[string]map[string]svgDecl{}, byID: map[string]map[string]svgDecl{}}
	for rule := range strings.SplitSeq(svgCSSComment.ReplaceAllString(css, svgCommentMark), "}") {
		selectors, block, found := strings.Cut(rule, "{")
		if !found {
			continue
		}
		paint := map[string]svgDecl{}
		decls := parseSVGDeclarations(block)
		for _, key := range svgCascadedProps {
			if decl, ok := decls[key]; ok && svgPaintAllowed(decl.value) {
				paint[key] = decl
			}
		}
		for selector := range strings.SplitSeq(selectors, ",") {
			selector = strings.TrimSpace(selector)
			target, name := rules.byTag, strings.ToLower(selector)
			if id, isID := strings.CutPrefix(selector, "#"); isID {
				target, name = rules.byID, id
			}
			if name == "" || strings.ContainsAny(name, " .:[>+~#*"+svgCommentMark) {
				continue
			}
			if target[name] == nil {
				target[name] = map[string]svgDecl{}
			}
			mergeDecl(target[name], paint)
		}
	}
	return rules
}

// resolveStyleValues replaces a declaration value that is currentColor.
func resolveStyleValues(block, color string) (string, bool) {
	changed := false
	var parts []string
	for pair := range strings.SplitSeq(block, ";") {
		if key, value, found := strings.Cut(pair, ":"); found && isCurrentColor(value) {
			pair, changed = key+":"+color, true
		}
		parts = append(parts, pair)
	}
	return strings.Join(parts, ";"), changed
}

// paintedStartTag returns the rewritten start tag, or nil when it needs no change.
func paintedStartTag(raw []byte, tag xml.StartElement, rules svgSelectorPaint, color string) []byte {
	matched := map[string]svgDecl{}
	mergeDecl(matched, rules.byTag[strings.ToLower(tag.Name.Local)])
	for _, attr := range tag.Attr {
		if attr.Name.Local == "id" {
			mergeDecl(matched, rules.byID[attr.Value])
		}
	}
	changed := len(matched) > 0
	attrs := make([]xml.Attr, 0, len(tag.Attr)+1)
	style := ""
	for _, attr := range tag.Attr {
		name := strings.ToLower(attr.Name.Local)
		switch {
		case attr.Name.Space == "" && name == "style":
			style = attr.Value
			continue
		case svgAttrOnlyProps[name] && matched[name].value != "":
			continue
		case svgCurrentAttr[name] && isCurrentColor(attr.Value):
			attr.Value = color
			changed = true
		}
		attrs = append(attrs, attr)
	}
	for key := range svgAttrOnlyProps {
		if decl, ok := matched[key]; ok {
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: key}, Value: resolvedPaint(decl.value, color)})
		}
	}
	style, resolved := resolveStyleValues(style, color)
	if !changed && !resolved {
		return nil
	}
	if len(matched) > 0 || style != "" {
		value := selectorStyle(matched, color, false) + strings.TrimRight(style, "; ") + ";" + selectorStyle(matched, color, true)
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "style"}, Value: value})
	}
	return serializeStartTag(tag.Name, attrs, bytes.HasSuffix(raw, []byte("/>")))
}

// selectorStyle writes the matched paint of one importance. The renderer reads
// later declarations last, so the normal block goes before the element's own
// style and the !important block after it.
func selectorStyle(matched map[string]svgDecl, color string, important bool) string {
	var b strings.Builder
	for _, key := range svgCascadedProps {
		decl, ok := matched[key]
		if !ok || decl.important != important || svgAttrOnlyProps[key] {
			continue
		}
		b.WriteString(key + ":" + resolvedPaint(decl.value, color) + ";")
	}
	return b.String()
}

func resolvedPaint(value, color string) string {
	if isCurrentColor(value) {
		return color
	}
	return value
}

func serializeStartTag(name xml.Name, attrs []xml.Attr, selfClosing bool) []byte {
	var b bytes.Buffer
	b.WriteByte('<')
	b.WriteString(qualifiedSVGName(name))
	for _, attr := range attrs {
		b.WriteByte(' ')
		b.WriteString(qualifiedSVGName(attr.Name))
		b.WriteString(`="`)
		// EscapeText cannot fail on a bytes.Buffer.
		if err := xml.EscapeText(&b, []byte(attr.Value)); err != nil {
			return nil
		}
		b.WriteByte('"')
	}
	if selfClosing {
		b.WriteByte('/')
	}
	b.WriteByte('>')
	return b.Bytes()
}

func qualifiedSVGName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}
