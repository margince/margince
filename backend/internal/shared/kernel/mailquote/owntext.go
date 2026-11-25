// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailquote

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

// preamblePattern is the From and To lines capture writes above a mail's text.
// Left in place, its From line reads as an Outlook reply header.
var preamblePattern = regexp.MustCompile(`^From:[^\n]*\nTo:[^\n]*\n\n`)

// MailKind is the activity kind whose body a mail program wrote.
const MailKind = "email"

// Preamble splits the capture preamble from the text under it. The preamble is
// empty when the body has none.
func Preamble(body string) (preamble, text string) {
	preamble = preamblePattern.FindString(body)
	return preamble, body[len(preamble):]
}

// OwnTextEnd is the index of the first line of quoted or forwarded mail, or
// len(lines) when the sender wrote every line.
//
// A line a mail program writes above such mail ends the text. A quote marked
// with ">" ends it only when nothing follows, so an inline reply stays whole.
//
// A forward passes its first quoted mail on as its own text. Outlook writes a
// forward and a reply alike, so only the subject tells them apart.
func OwnTextEnd(lines []string, subject string) int {
	for i := ForwardedBlockEnd(lines, subject); i < len(lines); i++ {
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

// ForwardedBlockEnd is the line after the header block of the mail a forward
// passes on. It is 0 for any other mail, and when the first quote opens no
// block. Gmail and Apple Mail write that block under a forward line, and
// Outlook writes it bare.
func ForwardedBlockEnd(lines []string, subject string) int {
	if !mailsubject.IsForward(subject) {
		return 0
	}
	header := forwardedHeader(lines)
	if header < 0 {
		return 0
	}
	end := header + 1
	for end < len(lines) && strings.TrimSpace(lines[end]) != "" && !startsQuote(lines, end) {
		end++
	}
	return end
}

// forwardedHeader is the line the forwarded mail's header block opens on, or
// -1 when there is none. An inline reply in the note is the note's own text.
func forwardedHeader(lines []string) int {
	for i := 0; i < len(lines); i++ {
		if !startsQuote(lines, i) && !OpensReplyHeader(lines[i], lines[i+1:]) {
			continue
		}
		if after, marked := markedRunEnd(lines, i); marked {
			if !hasText(lines[after:]) {
				return -1
			}
			i = after - 1
			continue
		}
		header := i
		for header < len(lines) && !OpensReplyHeader(lines[header], lines[header+1:]) {
			if header > i && strings.TrimSpace(lines[header]) != "" {
				return -1
			}
			header++
		}
		if header == len(lines) {
			return -1
		}
		return header
	}
	return -1
}

// OwnEnd is what a writer of an activity body stores beside it: how many
// characters, from the start, the sender wrote. The subject is the mail's own.
// It is nil for any kind but mail, and for a mail with no body.
func OwnEnd(kind, subject string, body *string) *int {
	if kind != MailKind || body == nil {
		return nil
	}
	chars := ownChars(*body, subject)
	return &chars
}

func ownChars(body, subject string) int {
	preamble, text := Preamble(body)
	lines := strings.Split(text, "\n")
	end := OwnTextEnd(lines, subject)
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
	if strings.HasPrefix(line, ">") || IsAttribution(line) {
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

func hasText(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}
