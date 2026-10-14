// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"bytes"
	"fmt"
	"mime/quotedprintable"
	"strings"
)

// headerLineLimit is the RFC 5322 recommended line length. The hard limit is
// 998 octets; folding to 78 keeps every relay and client inside both.
const headerLineLimit = 78

// encodedWordLimit is the RFC 2047 ceiling on one encoded word.
const encodedWordLimit = 75

// compose writes the whole message: headers, then the body as
// quoted-printable. Every relay carries that, with or without 8BITMIME, and it
// keeps each body line within 76 octets.
func (s SMTP) compose(to, subject, textBody, messageID string) ([]byte, error) {
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\n", s.FromAddress, to, foldedSubject(subject))
	if messageID != "" {
		fmt.Fprintf(&msg, "Message-ID: <%s>\r\n", messageID)
	}
	msg.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	body := quotedprintable.NewWriter(&msg)
	if _, err := body.Write([]byte(textBody)); err != nil {
		return nil, fmt.Errorf("mailer: encoding the body: %w", err)
	}
	if err := body.Close(); err != nil {
		return nil, fmt.Errorf("mailer: encoding the body: %w", err)
	}
	return msg.Bytes(), nil
}

// foldedSubject is the Subject value as header text. A short printable ASCII
// subject is written as it is. Anything else becomes RFC 2047 Q-encoded words,
// each on its own folded line, so no line passes headerLineLimit.
func foldedSubject(subject string) string {
	const name = len("Subject: ")
	if plainASCII(subject) && name+len(subject) <= headerLineLimit {
		return subject
	}
	var words []string
	var chunk strings.Builder
	limit := min(encodedWordLimit, headerLineLimit-name)
	for _, r := range subject {
		next := chunk.String() + string(r)
		if chunk.Len() > 0 && len(qWord(next)) > limit {
			words = append(words, qWord(chunk.String()))
			chunk.Reset()
			// A continuation line starts with one space.
			limit = min(encodedWordLimit, headerLineLimit-1)
		}
		chunk.WriteRune(r)
	}
	if chunk.Len() > 0 {
		words = append(words, qWord(chunk.String()))
	}
	return strings.Join(words, "\r\n ")
}

// qWord is one RFC 2047 Q-encoded word in UTF-8. Whole runes go in, so a word
// never splits a character.
func qWord(text string) string {
	var b strings.Builder
	b.WriteString("=?utf-8?q?")
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == ' ':
			b.WriteByte('_')
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '!', c == '*', c == '+', c == '-', c == '/':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "=%02X", c)
		}
	}
	b.WriteString("?=")
	return b.String()
}

// plainASCII reports whether text is printable ASCII and can stand in a header
// unencoded. An "=?" is encoded too, so a decoder never reads it as a word.
func plainASCII(text string) bool {
	if strings.Contains(text, "=?") {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < 0x20 || text[i] > 0x7E {
			return false
		}
	}
	return true
}
