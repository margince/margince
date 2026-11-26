// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a signature keeps beyond a message body: the table layout, the layout
// styles and the https images a mail client's signature editor writes. Every
// attribute and declaration is checked by kind and rewritten by this file, so
// nothing a sender typed reaches the wire unread.

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// attrWidth and attrHeight are both HTML attributes and CSS properties the
// policy bounds.
const (
	attrWidth  = "width"
	attrHeight = "height"
)

// attributeRules are the attributes one element keeps, each with the check its
// value must pass.
type attributeRules map[string]func(string) bool

var (
	cellAttributes = attributeRules{
		"colspan": isDimension, "rowspan": isDimension, attrWidth: isDimension, attrHeight: isDimension,
		"align": isOneOf("left", "center", "right", "justify"), "valign": isOneOf("top", "middle", "bottom", "baseline"),
	}
	signaturePolicy = sanitizePolicy{styled: true, extra: map[atom.Atom]attributeRules{
		atom.Table: {attrWidth: isDimension, "cellpadding": isDimension, "cellspacing": isDimension, "border": isDimension},
		atom.Thead: {}, atom.Tbody: {}, atom.Tr: {},
		atom.Td: cellAttributes, atom.Th: cellAttributes,
		atom.Div: {}, atom.Span: {},
		atom.Font: {"color": isColor},
		// src is checked by safeImageSrc, which decides whether the image stays.
		atom.Img: {attrWidth: isDimension, attrHeight: isDimension, "alt": func(string) bool { return true }},
	}}
)

// writeAttributes writes the attributes rules admits, in the order the source
// gave them, each once.
func writeAttributes(out *strings.Builder, node *html.Node, rules attributeRules) {
	seen := map[string]bool{}
	for _, attr := range node.Attr {
		key := strings.ToLower(attr.Key)
		check, ok := rules[key]
		value := strings.TrimSpace(attr.Val)
		if !ok || seen[key] || !check(value) {
			continue
		}
		seen[key] = true
		out.WriteString(" " + key + `="` + html.EscapeString(value) + `"`)
	}
}

// writeSignatureImage keeps an image whose source is https or the embedded
// logo, and drops any other image whole: an image has no text to unwrap.
func writeSignatureImage(out *strings.Builder, node *html.Node, rules attributeRules) {
	if embeddedLogo(node) {
		out.WriteString(signatureLogoTag)
		return
	}
	src, ok := safeImageSrc(node)
	if !ok {
		return
	}
	out.WriteString(`<img src="` + html.EscapeString(src) + `"`)
	writeAttributes(out, node, rules)
	out.WriteString(">")
}

// safeImageSrc admits an absolute https address with a host and no user info.
// http, data:, javascript:, a relative path and a cid: other than the logo
// fail. srcset is never written, because its addresses are not checked.
func safeImageSrc(node *html.Node) (string, bool) {
	for _, attr := range node.Attr {
		if !strings.EqualFold(attr.Key, "src") {
			continue
		}
		parsed, err := url.Parse(strings.TrimSpace(attr.Val))
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return "", false
		}
		return parsed.String(), true
	}
	return "", false
}

func embeddedLogo(node *html.Node) bool {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, "src") && attr.Val == signatureLogoSrc {
			return true
		}
	}
	return false
}

// styleChecks are the declarations a signature keeps, each with the check its
// value must pass. A property not listed, and a value that fails, is dropped.
var styleChecks = map[string]func(string) bool{
	"color": isColor, "background-color": isColor,
	"font-family": isFontFamily, "font-size": fontSize.MatchString,
	"font-weight":     isOneOf("normal", "bold", "bolder", "lighter", "100", "200", "300", "400", "500", "600", "700", "800", "900"),
	"font-style":      isOneOf("normal", "italic", "oblique"),
	"line-height":     func(v string) bool { return v == "normal" || isLength(v) || unitless.MatchString(v) },
	"text-align":      isOneOf("left", "right", "center", "justify", "start", "end"),
	"text-decoration": isOneOf("none", "underline", "line-through", "overline"),
	"vertical-align":  isOneOf("top", "middle", "bottom", "baseline", "text-top", "text-bottom"),
	attrWidth:         isSize, "max-width": isSize, attrHeight: isSize,
	"padding": isBox, "padding-top": isLength, "padding-right": isLength, "padding-bottom": isLength, "padding-left": isLength,
	"margin": isBox, "margin-top": isSize, "margin-right": isSize, "margin-bottom": isSize, "margin-left": isSize,
	"border": isBorder, "border-top": isBorder, "border-right": isBorder, "border-bottom": isBorder, "border-left": isBorder,
	"border-color": isColor, "border-width": isLength, "border-style": isBorderStyle,
	"border-radius": isBox, "border-spacing": isBox,
	"border-collapse": isOneOf("collapse", "separate"),
}

var (
	// The size bounds keep text readable: a 0px or 1px span hides words from
	// the reader while a mail client still sends them.
	fontSize  = regexp.MustCompile(`^(([89]|[12][0-9]|3[0-2])px|([6-9]|1[0-9]|2[0-4])(\.5)?pt)$`)
	length    = regexp.MustCompile(`^(0|[0-9]{1,4}(\.[0-9]{1,2})?(px|pt|em|%))$`)
	unitless  = regexp.MustCompile(`^[0-9](\.[0-9]{1,2})?$`)
	dimension = regexp.MustCompile(`^[0-9]{1,4}%?$`)
	// Three or six digits: the four- and eight-digit forms carry an alpha.
	hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	rgbColor = regexp.MustCompile(`^rgba?\(\s*[0-9]{1,3}%?\s*,\s*[0-9]{1,3}%?\s*,\s*[0-9]{1,3}%?\s*(,\s*([01]|0?\.[0-9]{1,3})\s*)?\)$`)
)

func safeStyle(node *html.Node) string {
	var kept []string
	for _, attr := range node.Attr {
		if !strings.EqualFold(attr.Key, "style") {
			continue
		}
		for decl := range strings.SplitSeq(attr.Val, ";") {
			prop, value, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}
			prop, value = strings.ToLower(strings.TrimSpace(prop)), strings.TrimSpace(value)
			if check, known := styleChecks[prop]; known && check(value) {
				kept = append(kept, prop+":"+value)
			}
		}
	}
	return strings.Join(withoutHiddenText(kept), ";")
}

// withoutHiddenText drops a text colour equal to the background's, which
// would hide the element's words from its reader.
func withoutHiddenText(kept []string) []string {
	colours := map[string]string{}
	for _, decl := range kept {
		prop, value, _ := strings.Cut(decl, ":")
		colours[prop] = strings.ToLower(strings.ReplaceAll(value, " ", ""))
	}
	text, hasText := colours["color"]
	if !hasText || text != colours["background-color"] {
		return kept
	}
	out := kept[:0]
	for _, decl := range kept {
		if !strings.HasPrefix(decl, "color:") {
			out = append(out, decl)
		}
	}
	return out
}

func isOneOf(values ...string) func(string) bool {
	return func(v string) bool {
		for _, allowed := range values {
			if strings.EqualFold(v, allowed) {
				return true
			}
		}
		return false
	}
}

func isDimension(v string) bool { return dimension.MatchString(v) }

func isLength(v string) bool { return length.MatchString(v) }

func isSize(v string) bool { return v == "auto" || isLength(v) }

// namedColors are the CSS colour keywords. A keyword outside it, such as
// currentColor, inherit or transparent, takes its value from somewhere a
// reader of the signature cannot see.
var namedColors = func() map[string]bool {
	out := map[string]bool{}
	for name := range strings.FieldsSeq(
		"aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet " +
			"brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan " +
			"darkblue darkcyan darkgoldenrod darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen " +
			"darkorange darkorchid darkred darksalmon darkseagreen darkslateblue darkslategray darkslategrey " +
			"darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue firebrick floralwhite " +
			"forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew " +
			"hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue " +
			"lightcoral lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon " +
			"lightseagreen lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime " +
			"limegreen linen magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple " +
			"mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue " +
			"mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid " +
			"palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum " +
			"powderblue purple rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen " +
			"seashell sienna silver skyblue slateblue slategray slategrey snow springgreen steelblue tan teal " +
			"thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen ") {
		out[name] = true
	}
	return out
}()

// minAlpha is the faintest rgba() a signature keeps, so its words stay legible.
const minAlpha = 0.5

func isColor(v string) bool {
	if hexColor.MatchString(v) || namedColors[strings.ToLower(v)] {
		return true
	}
	if !rgbColor.MatchString(v) {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(v, ")"), ",")
	if len(parts) < 4 {
		return true
	}
	alpha, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	return err == nil && alpha >= minAlpha
}

// fontFamilies are the faces a signature may name: common ones every mail
// client has, so a family cannot point a client at a remote font.
var fontFamilies = map[string]bool{
	"serif": true, "sans-serif": true, "monospace": true, "arial": true, "helvetica": true,
	"georgia": true, "times new roman": true, "verdana": true,
}

func isFontFamily(v string) bool {
	for family := range strings.SplitSeq(v, ",") {
		if !fontFamilies[strings.ToLower(strings.Trim(strings.TrimSpace(family), `"'`))] {
			return false
		}
	}
	return true
}

// isBox admits the one to four sizes padding, margin and their kin take.
func isBox(v string) bool {
	parts := strings.Fields(v)
	if len(parts) == 0 || len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if !isSize(part) {
			return false
		}
	}
	return true
}

var isBorderStyle = isOneOf("none", "hidden", "solid", "dashed", "dotted", "double")

// isBorder admits a shorthand of a width, a style and a colour, in any order.
func isBorder(v string) bool {
	parts := cssTokens(v)
	if len(parts) == 0 || len(parts) > 3 {
		return false
	}
	for _, part := range parts {
		if !isLength(part) && !isBorderStyle(part) && !isColor(part) {
			return false
		}
	}
	return true
}

// cssTokens splits a value at the spaces outside parentheses, so
// "1px solid rgb(1, 2, 3)" is three tokens.
func cssTokens(v string) []string {
	var tokens []string
	depth, start := 0, -1
	for i, r := range v {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ' ' && depth == 0:
			if start >= 0 {
				tokens = append(tokens, v[start:i])
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		tokens = append(tokens, v[start:])
	}
	return tokens
}
