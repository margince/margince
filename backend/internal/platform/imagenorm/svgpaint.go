// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"maps"
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
	svgCurrentColor = regexp.MustCompile(`(?i)currentcolor`)
	// svgSafePaint admits colour words, hex, rgb(), hsl() and url(#id).
	// Any other url() is refused, so the rewrite names no external resource.
	svgSafePaint   = regexp.MustCompile(`^(?:[#A-Za-z0-9 .,%+-]+|(?:rgba?|hsla?)\([0-9 .,%+-]+\)|url\(#[A-Za-z0-9_.:-]+\))$`)
	svgCSSComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	svgCurrentAttr = map[string]bool{"fill": true, "stroke": true, "stop-color": true, "flood-color": true, "lighting-color": true}
	// svgCascadedProps are the only properties a selector rule is inlined for.
	svgCascadedProps = map[string]bool{"fill": true, "stroke": true}
)

// svgSelectorPaint holds the winning fill and stroke per tag name and per id.
type svgSelectorPaint struct {
	byTag, byID map[string]map[string]string
}

type svgPaintScan struct {
	color string
	css   strings.Builder
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
			if styleDepth > 0 && svgCurrentColor.Match(t) {
				replacement = svgCurrentColor.ReplaceAll(src[start:end], []byte(scan.color))
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
			if value, ok := parseSVGDeclarations(attr.Value)["color"]; ok {
				color = value
			}
		}
	}
	if color == "" || svgCurrentColor.MatchString(color) || !svgPaintAllowed(color) || strings.EqualFold(color, "inherit") {
		return svgDefaultColor
	}
	return color
}

// parseSVGDeclarations reads "a:b;c:d" into a map; the last declaration of a
// property wins and !important is dropped.
func parseSVGDeclarations(block string) map[string]string {
	decls := map[string]string{}
	for pair := range strings.SplitSeq(block, ";") {
		key, value, found := strings.Cut(pair, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "!important"))
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" && value != "" {
			decls[key] = value
		}
	}
	return decls
}

// parseSVGSelectors keeps the fill and stroke of rules whose selector is a bare
// tag name or #id. Class rules stay with the renderer, which applies them last.
// That order already ranks a class above a tag, as CSS does.
func parseSVGSelectors(css string) svgSelectorPaint {
	rules := svgSelectorPaint{byTag: map[string]map[string]string{}, byID: map[string]map[string]string{}}
	for rule := range strings.SplitSeq(svgCSSComment.ReplaceAllString(css, ""), "}") {
		selectors, block, found := strings.Cut(rule, "{")
		if !found {
			continue
		}
		paint := map[string]string{}
		for key, value := range parseSVGDeclarations(block) {
			if svgCascadedProps[key] && svgPaintAllowed(value) {
				paint[key] = value
			}
		}
		for selector := range strings.SplitSeq(selectors, ",") {
			selector = strings.TrimSpace(selector)
			target, name := rules.byTag, strings.ToLower(selector)
			if id, isID := strings.CutPrefix(selector, "#"); isID {
				target, name = rules.byID, id
			}
			if name == "" || strings.ContainsAny(name, " .:[>+~#*") {
				continue
			}
			if target[name] == nil {
				target[name] = map[string]string{}
			}
			maps.Copy(target[name], paint)
		}
	}
	return rules
}

// paintedStartTag returns the rewritten start tag, or nil when it needs no change.
func paintedStartTag(raw []byte, tag xml.StartElement, rules svgSelectorPaint, color string) []byte {
	inlined := map[string]string{}
	maps.Copy(inlined, rules.byTag[strings.ToLower(tag.Name.Local)])
	for _, attr := range tag.Attr {
		if attr.Name.Local == "id" {
			maps.Copy(inlined, rules.byID[attr.Value])
		}
	}
	changed := len(inlined) > 0
	attrs := make([]xml.Attr, 0, len(tag.Attr)+1)
	style := ""
	for _, attr := range tag.Attr {
		name := strings.ToLower(attr.Name.Local)
		switch {
		case attr.Name.Space == "" && name == "style":
			style = attr.Value
			continue
		case svgCurrentAttr[name] && svgCurrentColor.MatchString(attr.Value):
			attr.Value = svgCurrentColor.ReplaceAllString(attr.Value, color)
			changed = true
		}
		attrs = append(attrs, attr)
	}
	if style != "" && svgCurrentColor.MatchString(style) {
		style = svgCurrentColor.ReplaceAllString(style, color)
		changed = true
	}
	if !changed {
		return nil
	}
	if len(inlined) > 0 || style != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "style"}, Value: inlinedStyle(inlined, color) + style})
	}
	return serializeStartTag(tag.Name, attrs, bytes.HasSuffix(raw, []byte("/>")))
}

// inlinedStyle puts the selector paint first so a declaration of the element's
// own style, which the renderer reads later, overrides it.
func inlinedStyle(paint map[string]string, color string) string {
	var b strings.Builder
	for _, key := range []string{"fill", "stroke"} {
		if value, ok := paint[key]; ok {
			b.WriteString(key + ":" + svgCurrentColor.ReplaceAllString(value, color) + ";")
		}
	}
	return b.String()
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
