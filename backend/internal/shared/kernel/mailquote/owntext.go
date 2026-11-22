// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailquote

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// preamblePattern is the From and To lines capture writes above a mail's text.
// Left in place, its From line reads as an Outlook reply header.
var preamblePattern = regexp.MustCompile(`^From:[^\n]*\nTo:[^\n]*\n\n`)

// forwardHeaderLine is one field of the header block under a forward line.
var forwardHeaderLine = regexp.MustCompile(`^\*{0,2}[^\s:>*][^:]{0,30}:`)

// MailKind is the activity kind whose body a mail program wrote.
const MailKind = "email"

// Preamble splits the capture preamble from the text under it. The preamble is
// empty when the body has none.
func Preamble(body string) (preamble, text string) {
	preamble = preamblePattern.FindString(body)
	return preamble, body[len(preamble):]
}

// OwnTextEnd is the index of the first line of quoted earlier mail, or
// len(lines) when the sender wrote or forwarded every line.
//
// A line a mail program writes above quoted mail ends the text. A quote marked
// with ">" ends it only when nothing follows, so an inline reply stays whole.
func OwnTextEnd(lines []string) int {
	for i := 0; i < len(lines); i++ {
		if IsForward(lines[i]) {
			i = forwardedTextStart(lines, i+1) - 1
			continue
		}
		if !startsQuote(lines, i) {
			continue
		}
		after, marked := markedRunEnd(lines, i)
		if !marked {
			return i
		}
		if !hasText(lines[after:]) {
			return attributedStart(lines, i)
		}
		i = after - 1
	}
	return len(lines)
}

// OwnEnd is what a writer of an activity body stores beside it: how many
// characters, from the start, the sender wrote. It is nil for any kind but mail,
// and for a mail with no body.
func OwnEnd(kind string, body *string) *int {
	if kind != MailKind || body == nil {
		return nil
	}
	chars := ownChars(*body)
	return &chars
}

func ownChars(body string) int {
	preamble, text := Preamble(body)
	lines := strings.Split(text, "\n")
	end := OwnTextEnd(lines)
	own := len(preamble)
	for _, line := range lines[:end] {
		own += len(line) + 1
	}
	// A body that is only a quote keeps every line, as the display does.
	if end == len(lines) || strings.TrimSpace(body[len(preamble):own]) == "" {
		return utf8.RuneCountInString(body)
	}
	return utf8.RuneCountInString(body[:own])
}

func startsQuote(lines []string, index int) bool {
	line := lines[index]
	if strings.HasPrefix(line, ">") || IsReply(line) {
		return true
	}
	// The sent-date field below separates an Outlook block from prose opening "Von:".
	return index > 0 && OpensReplyHeader(line, lines[index+1:])
}

// markedRunEnd reads the ">" lines a quote at index opens, and returns the
// index after them. marked is false when the quote carries no ">" lines.
func markedRunEnd(lines []string, index int) (after int, marked bool) {
	first := index
	if !strings.HasPrefix(lines[first], ">") {
		for first++; first < len(lines) && strings.TrimSpace(lines[first]) == ""; first++ {
		}
		if first == len(lines) || !strings.HasPrefix(lines[first], ">") {
			return 0, false
		}
	}
	for after = first; after < len(lines); after++ {
		if !strings.HasPrefix(lines[after], ">") && strings.TrimSpace(lines[after]) != "" {
			break
		}
	}
	return after, true
}

// attributedStart moves a quote's start up to the line that introduces it.
func attributedStart(lines []string, index int) int {
	if strings.HasPrefix(lines[index], ">") && index > 0 && strings.HasSuffix(strings.TrimSpace(lines[index-1]), ":") {
		return index - 1
	}
	return index
}

// forwardedTextStart is the index where a forwarded mail's own lines are read
// from. It passes the header block, whose From and Date fields would read as an
// Outlook reply header, and a mail forwarded with ">" on every line.
func forwardedTextStart(lines []string, from int) int {
	for from < len(lines) && strings.TrimSpace(lines[from]) == "" {
		from++
	}
	end := from
	for end < len(lines) && end-from < quotedHeaderReach && forwardHeaderLine.MatchString(lines[end]) {
		end++
	}
	for end < len(lines) && (strings.HasPrefix(lines[end], ">") || strings.TrimSpace(lines[end]) == "") {
		end++
	}
	return end
}

func hasText(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}
