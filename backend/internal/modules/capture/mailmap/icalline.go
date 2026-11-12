// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import "strings"

// icalLine is one unfolded iCalendar content line: NAME;PARAM=VALUE:VALUE.
type icalLine struct {
	name   string
	params map[string]string
	value  string
}

// param is a parameter's value, unquoted, or "".
func (l icalLine) param(name string) string { return l.params[strings.ToUpper(name)] }

// is says whether the line is name, case aside.
func (l icalLine) is(name string) bool { return strings.EqualFold(l.name, name) }

// parseICalLine splits a content line at the first colon outside a quoted
// parameter. Outlook quotes names holding a comma or colon
// (CN="Weber, Anna"), and a plain cut at the first colon read that as the value.
func parseICalLine(line string) (icalLine, bool) {
	inQuote := false
	split := -1
	for i, r := range line {
		if r == '"' {
			inQuote = !inQuote
		}
		if r == ':' && !inQuote {
			split = i
			break
		}
	}
	if split < 0 {
		return icalLine{}, false
	}
	head := splitOutsideQuotes(line[:split], ';')
	out := icalLine{name: strings.TrimSpace(head[0]), value: line[split+1:], params: map[string]string{}}
	for _, param := range head[1:] {
		key, value, found := strings.Cut(param, "=")
		if found {
			out.params[strings.ToUpper(strings.TrimSpace(key))] = strings.Trim(value, `"`)
		}
	}
	return out, true
}

func splitOutsideQuotes(text string, sep rune) []string {
	var out []string
	inQuote := false
	start := 0
	for i, r := range text {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == sep && !inQuote:
			out = append(out, text[start:i])
			start = i + 1
		}
	}
	return append(out, text[start:])
}

// icalLines are a body's content lines, unfolded and split; a line without a
// colon is no property and is left out.
func icalLines(body []byte) []icalLine {
	var out []icalLine
	for _, raw := range unfoldICalendar(body) {
		if line, ok := parseICalLine(strings.TrimRight(raw, "\r")); ok {
			out = append(out, line)
		}
	}
	return out
}

// icalText undoes RFC 5545 TEXT escaping: \n, \, \; and \\.
func icalText(value string) string {
	var b strings.Builder
	escaped := false
	for _, r := range value {
		switch {
		case escaped && (r == 'n' || r == 'N'):
			b.WriteRune('\n')
			escaped = false
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// icalAddress is the address of a CAL-ADDRESS value: mailto:a@b, lower-cased.
func icalAddress(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > len("mailto:") && strings.EqualFold(value[:len("mailto:")], "mailto:") {
		value = value[len("mailto:"):]
	}
	return strings.ToLower(strings.TrimSpace(value))
}
