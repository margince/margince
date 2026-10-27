// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	contentTypeJSON = "application/json"
	contentTypeText = "text/plain"
	contentTypeForm = "application/x-www-form-urlencoded"

	fieldBodyTemplate    = "body_template"
	fieldBodyContentType = "body_content_type"

	maxTemplateBytes = 16 << 10
	maxBodyBytes     = 256 << 10
	maxPlaceholders  = 500
)

// templateRoots are the public envelope's fields: a template reads the
// envelope a subscriber would have received, never anything wider.
var templateRoots = []string{"event_id", "type", "version", "occurred_at", "actor", "entity", "correlation_id", "data"}

var errBodyTooLarge = fmt.Errorf("the rendered body exceeds %d KiB", maxBodyBytes>>10)

// bodyTemplate is a parsed `{{ path }}` template. In JSON mode the parser
// fixes where each placeholder may sit: `{{ x }}` inside a JSON string,
// `{{ x | json }}` outside one. So any event renders valid JSON.
type bodyTemplate struct {
	contentType string
	parts       []templatePart
}

type templatePart struct {
	literal string
	path    []string // nil for a literal part
	asJSON  bool
	// currency is the path `| money` reads the currency code from; nil
	// without that filter.
	currency []string
}

// normalizeContentType resolves the stored or requested content type; an
// empty value is the JSON default.
func normalizeContentType(contentType string) (string, error) {
	switch contentType {
	case "":
		return contentTypeJSON, nil
	case contentTypeJSON, contentTypeText, contentTypeForm:
		return contentType, nil
	}
	return "", &BadInputError{
		Field:  fieldBodyContentType,
		Reason: fmt.Sprintf("must be one of %s, %s or %s", contentTypeJSON, contentTypeText, contentTypeForm),
	}
}

func parseBodyTemplate(src, contentType string) (*bodyTemplate, error) {
	ct, err := normalizeContentType(contentType)
	if err != nil {
		return nil, err
	}
	if len(src) > maxTemplateBytes {
		return nil, templateError(fmt.Sprintf("is longer than %d KiB", maxTemplateBytes>>10))
	}
	if strings.TrimSpace(src) == "" {
		return nil, templateError("is empty; clear it to send the standard envelope")
	}
	t := &bodyTemplate{contentType: ct}
	for offset := 0; offset < len(src); {
		open := strings.Index(src[offset:], "{{")
		if open < 0 {
			t.parts = append(t.parts, templatePart{literal: src[offset:]})
			break
		}
		open += offset
		if open > offset {
			t.parts = append(t.parts, templatePart{literal: src[offset:open]})
		}
		end := strings.Index(src[open:], "}}")
		if end < 0 {
			return nil, templateErrorAt(src, open, "the placeholder is never closed with }}")
		}
		part, err := parsePlaceholder(src[open+2 : open+end])
		if err != nil {
			return nil, templateErrorAt(src, open, err.Error())
		}
		t.parts = append(t.parts, part)
		offset = open + end + 2
	}
	if placeholders := len(t.parts) - countLiterals(t.parts); placeholders > maxPlaceholders {
		return nil, templateError(fmt.Sprintf("has %d placeholders; the limit is %d", placeholders, maxPlaceholders))
	}
	if ct == contentTypeJSON {
		if err := t.checkJSONPositions(src); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func countLiterals(parts []templatePart) int {
	n := 0
	for _, p := range parts {
		if p.path == nil {
			n++
		}
	}
	return n
}

func parsePlaceholder(inner string) (templatePart, error) {
	expr, filter, hasFilter := strings.Cut(inner, "|")
	part := templatePart{}
	var err error
	if hasFilter {
		if err = part.applyFilter(strings.Fields(filter)); err != nil {
			return part, err
		}
	}
	if part.path, err = parseFieldPath(expr, "the placeholder names no field"); err != nil {
		return part, err
	}
	return part, nil
}

// applyFilter reads `json`, or `money <currency path>`.
func (p *templatePart) applyFilter(words []string) error {
	switch {
	case len(words) == 1 && words[0] == "json":
		p.asJSON = true
		return nil
	case len(words) > 0 && words[0] == "money":
		if len(words) != 2 {
			return errors.New("money needs the field that holds the currency, as in | money data.currency_at_change")
		}
		currency, err := parseFieldPath(words[1], "money names no currency field")
		p.currency = currency
		return err
	case len(words) == 0:
		return errors.New("the placeholder names no filter after |")
	}
	return fmt.Errorf("%q is not a filter; the filters are json and money", words[0])
}

func parseFieldPath(expr, emptyReason string) ([]string, error) {
	expr = strings.TrimSpace(expr)
	path := strings.Split(expr, ".")
	if expr == "" || slices.Contains(path, "") {
		return nil, errors.New(emptyReason)
	}
	if !slices.Contains(templateRoots, path[0]) {
		return nil, fmt.Errorf("%q is not a field of the event; use one of %s", path[0], strings.Join(templateRoots, ", "))
	}
	return path, nil
}

// jsonLexer tracks where a scan of JSON text stands: inside a string, after a
// backslash, or inside a \u escape that still needs hexLeft digits.
type jsonLexer struct {
	inString, escaped bool
	hexLeft           int
}

func (l *jsonLexer) scan(text string) {
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
		case l.hexLeft > 0:
			l.hexLeft--
		case l.escaped:
			l.escaped = false
			if c == 'u' {
				l.hexLeft = 4
			}
		case l.inString && c == '\\':
			l.escaped = true
		case c == '"':
			l.inString = !l.inString
		}
	}
}

// refusal says why a placeholder may not stand where the lexer is, or "".
func (l *jsonLexer) refusal(asJSON bool) string {
	switch {
	case l.escaped:
		return "a backslash escapes the start of the placeholder; remove it"
	case l.hexLeft > 0:
		return "the placeholder sits inside a \\u escape; finish the four hex digits first"
	case l.inString && asJSON:
		return "| json writes a whole JSON value, so it cannot sit inside a JSON string; drop | json or the quotes"
	case !l.inString && !asJSON:
		return "a placeholder outside a JSON string needs | json, or put it inside quotes"
	}
	return ""
}

// checkJSONPositions walks the literal text as a JSON lexer would, so it
// knows whether each placeholder sits inside a string. It then proves the
// skeleton is JSON, with every placeholder standing in for a value.
func (t *bodyTemplate) checkJSONPositions(src string) error {
	var skeleton strings.Builder
	var lex jsonLexer
	offset := 0
	for _, p := range t.parts {
		if p.path == nil {
			lex.scan(p.literal)
			skeleton.WriteString(p.literal)
			offset += len(p.literal)
			continue
		}
		at := strings.Index(src[offset:], "{{") + offset
		if why := lex.refusal(p.asJSON); why != "" {
			return templateErrorAt(src, at, why)
		}
		if !lex.inString {
			skeleton.WriteString("null")
		}
		offset = strings.Index(src[at:], "}}") + at + 2
	}
	if !json.Valid([]byte(skeleton.String())) {
		return templateError("is not valid JSON once the placeholders are filled in; check the quotes, commas and brackets")
	}
	return nil
}

func templateError(reason string) *BadInputError {
	return &BadInputError{Field: fieldBodyTemplate, Reason: "the template " + reason}
}

func templateErrorAt(src string, offset int, reason string) *BadInputError {
	line := strings.Count(src[:offset], "\n") + 1
	col := offset - strings.LastIndex(src[:offset], "\n")
	return &BadInputError{Field: fieldBodyTemplate, Reason: fmt.Sprintf("line %d, column %d: %s", line, col, reason)}
}

// render fills the template from a marshalled public envelope.
func (t *bodyTemplate) render(envelope []byte) ([]byte, error) {
	doc := newRawDoc(envelope)
	var out bytes.Buffer
	for _, p := range t.parts {
		if p.path == nil {
			out.WriteString(p.literal)
		} else if err := t.writePart(&out, doc, p); err != nil {
			return nil, err
		}
		if out.Len() > maxBodyBytes {
			return nil, errBodyTooLarge
		}
	}
	return out.Bytes(), nil
}

func (t *bodyTemplate) writePart(out *bytes.Buffer, doc *rawDoc, p templatePart) error {
	if p.currency != nil {
		return t.writeText(out, moneyText(doc.lookup(p.path), doc.lookup(p.currency)))
	}
	text := valueText(doc.lookup(p.path), p.asJSON)
	if p.asJSON && t.contentType == contentTypeJSON {
		out.WriteString(text)
		return nil
	}
	return t.writeText(out, text)
}

// writeText writes text the way the content type escapes it; in JSON it
// stands inside a string, which the parser has already made sure of.
func (t *bodyTemplate) writeText(out *bytes.Buffer, text string) error {
	switch t.contentType {
	case contentTypeForm:
		out.WriteString(url.QueryEscape(text))
	case contentTypeJSON:
		quoted, err := marshalNoHTMLEscape(text)
		if err != nil {
			return err
		}
		out.Write(quoted[1 : len(quoted)-1])
	default:
		out.WriteString(text)
	}
	return nil
}

// moneyText renders an integer count of minor units in its currency, as
// "56,830.20 USD". It is empty unless both halves are present and well formed,
// because an amount without its currency cannot say how many digits are cents.
func moneyText(amount, currency json.RawMessage) string {
	minor, err := strconv.ParseInt(string(bytes.TrimSpace(amount)), 10, 64)
	if err != nil {
		return ""
	}
	var code string
	if json.Unmarshal(currency, &code) != nil || !values.ValidCurrency(code) {
		return ""
	}
	whole, fraction, hasFraction := strings.Cut(values.MajorUnits(minor, code), ".")
	text := groupThousands(whole)
	if hasFraction {
		text += "." + fraction
	}
	return text + " " + code
}

// groupThousands puts a comma between each group of three digits of a whole
// number, keeping a leading minus sign in front.
func groupThousands(whole string) string {
	sign, digits := "", whole
	if strings.HasPrefix(whole, "-") {
		sign, digits = "-", whole[1:]
	}
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return sign + grouped.String()
}

// valueText is a value as text: a string's contents, any other value's
// compact JSON. A missing value is empty, or null when written as JSON.
func valueText(raw json.RawMessage, asJSON bool) string {
	if len(raw) == 0 || (!asJSON && string(raw) == "null") {
		if asJSON {
			return "null"
		}
		return ""
	}
	if !asJSON && raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return string(raw)
	}
	return compact.String()
}

func marshalNoHTMLEscape(s string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// rawDoc walks dot paths through raw JSON without decoding numbers, so a value
// keeps the exact text the envelope carried. Each object or array is decoded
// once per render, however many placeholders read through it.
type rawDoc struct {
	root    json.RawMessage
	objects map[string]map[string]json.RawMessage
	arrays  map[string][]json.RawMessage
}

func newRawDoc(root json.RawMessage) *rawDoc {
	return &rawDoc{root: root, objects: map[string]map[string]json.RawMessage{}, arrays: map[string][]json.RawMessage{}}
}

// lookup resolves a path, or returns nil when any segment misses.
func (d *rawDoc) lookup(path []string) json.RawMessage {
	cur := d.root
	for depth, seg := range path {
		cur = d.child(strings.Join(path[:depth], "."), cur, seg)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// child is one step down from the value at prefix, decoding that value at
// most once.
func (d *rawDoc) child(prefix string, value json.RawMessage, seg string) json.RawMessage {
	value = bytes.TrimSpace(value)
	switch {
	case len(value) > 0 && value[0] == '{':
		obj, seen := d.objects[prefix]
		if !seen {
			if json.Unmarshal(value, &obj) != nil {
				obj = nil
			}
			d.objects[prefix] = obj
		}
		return obj[seg]
	case len(value) > 0 && value[0] == '[':
		arr, seen := d.arrays[prefix]
		if !seen {
			if json.Unmarshal(value, &arr) != nil {
				arr = nil
			}
			d.arrays[prefix] = arr
		}
		i, err := strconv.Atoi(seg)
		if err != nil || i < 0 || i >= len(arr) {
			return nil
		}
		return arr[i]
	}
	return nil
}
