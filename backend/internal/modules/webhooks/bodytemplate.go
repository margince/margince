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
)

const (
	contentTypeJSON = "application/json"
	contentTypeText = "text/plain"
	contentTypeForm = "application/x-www-form-urlencoded"

	fieldBodyTemplate    = "body_template"
	fieldBodyContentType = "body_content_type"

	maxTemplateBytes = 16 << 10
	maxBodyBytes     = 256 << 10
)

// templateRoots are the public envelope's fields: a template reads the
// envelope a subscriber would have received, never anything wider.
var templateRoots = []string{"event_id", "type", "version", "occurred_at", "actor", "entity", "correlation_id", "data"}

var errBodyTooLarge = fmt.Errorf("the rendered body exceeds %d KiB", maxBodyBytes>>10)

// bodyTemplate is a parsed `{{ path }}` template. In JSON mode every
// placeholder's position is fixed at parse time — `{{ x }}` inside a JSON
// string, `{{ x | json }}` outside one — so any event renders valid JSON.
type bodyTemplate struct {
	contentType string
	parts       []templatePart
}

type templatePart struct {
	literal string
	path    []string // nil for a literal part
	asJSON  bool
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
	return "", &BadInputError{Field: fieldBodyContentType,
		Reason: fmt.Sprintf("must be one of %s, %s or %s", contentTypeJSON, contentTypeText, contentTypeForm)}
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
	if ct == contentTypeJSON {
		if err := t.checkJSONPositions(src); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func parsePlaceholder(inner string) (templatePart, error) {
	expr, filter, hasFilter := strings.Cut(inner, "|")
	part := templatePart{}
	if hasFilter {
		if name := strings.TrimSpace(filter); name != "json" {
			return part, fmt.Errorf("%q is not a filter; the only filter is json", name)
		}
		part.asJSON = true
	}
	expr = strings.TrimSpace(expr)
	part.path = strings.Split(expr, ".")
	if expr == "" || slices.Contains(part.path, "") {
		return part, errors.New("the placeholder names no field")
	}
	if !slices.Contains(templateRoots, part.path[0]) {
		return part, fmt.Errorf("%q is not a field of the event; use one of %s", part.path[0], strings.Join(templateRoots, ", "))
	}
	return part, nil
}

// checkJSONPositions walks the literal text as a JSON lexer would, so it
// knows whether each placeholder sits inside a string, and then proves the
// literal skeleton is JSON with every placeholder standing in for a value.
func (t *bodyTemplate) checkJSONPositions(src string) error {
	var skeleton strings.Builder
	inString, escaped, offset := false, false, 0
	for _, p := range t.parts {
		if p.path == nil {
			for i := 0; i < len(p.literal); i++ {
				switch c := p.literal[i]; {
				case escaped:
					escaped = false
				case inString && c == '\\':
					escaped = true
				case c == '"':
					inString = !inString
				}
			}
			skeleton.WriteString(p.literal)
			offset += len(p.literal)
			continue
		}
		at := strings.Index(src[offset:], "{{") + offset
		switch {
		case escaped:
			return templateErrorAt(src, at, "a backslash escapes the start of the placeholder; remove it")
		case inString && p.asJSON:
			return templateErrorAt(src, at, "| json writes a whole JSON value, so it cannot sit inside a JSON string; drop | json or the quotes")
		case !inString && !p.asJSON:
			return templateErrorAt(src, at, "a placeholder outside a JSON string needs | json, or put it inside quotes")
		case !inString:
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
	var out bytes.Buffer
	for _, p := range t.parts {
		if p.path == nil {
			out.WriteString(p.literal)
		} else if err := t.writeValue(&out, lookup(envelope, p.path), p.asJSON); err != nil {
			return nil, err
		}
		if out.Len() > maxBodyBytes {
			return nil, errBodyTooLarge
		}
	}
	return out.Bytes(), nil
}

func (t *bodyTemplate) writeValue(out *bytes.Buffer, raw json.RawMessage, asJSON bool) error {
	text := valueText(raw, asJSON)
	switch {
	case t.contentType == contentTypeForm:
		out.WriteString(url.QueryEscape(text))
	case t.contentType == contentTypeJSON && !asJSON:
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

// lookup walks a dot path through raw JSON without decoding numbers, so a
// value keeps the exact text the envelope carried. A miss returns nil.
func lookup(doc json.RawMessage, path []string) json.RawMessage {
	cur := doc
	for _, seg := range path {
		cur = bytes.TrimSpace(cur)
		switch {
		case len(cur) > 0 && cur[0] == '{':
			var obj map[string]json.RawMessage
			if json.Unmarshal(cur, &obj) != nil {
				return nil
			}
			cur = obj[seg]
		case len(cur) > 0 && cur[0] == '[':
			var arr []json.RawMessage
			i, err := strconv.Atoi(seg)
			if json.Unmarshal(cur, &arr) != nil || err != nil || i < 0 || i >= len(arr) {
				return nil
			}
			cur = arr[i]
		default:
			return nil
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}
