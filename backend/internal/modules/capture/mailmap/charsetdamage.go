// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"
	"unicode"

	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
)

// CharsetRecovery is what a stored mail should say now. Subject is nil when the
// stored subject is not the shape the parser before charset decoding left.
// BodyChanged is false when the stored body is not that shape; Body is then the
// stored one.
type CharsetRecovery struct {
	Subject      *string
	Body         string
	BodyChanged  bool
	Completeness capture.BodyCompleteness
}

// RecoverCharsetDamage re-derives the subject and body of a mail stored before
// header words and body parts in legacy charsets were decoded. It answers ok
// only for a value equal to what the older parser stored from raw, so a value
// a member wrote is never touched.
// That parser kept an encoded word as the subject. It stored a question mark or
// U+FFFD per byte of a non-UTF-8 body part. A multi-byte charset matches no
// stored shape, so its mail stays as it is.
func RecoverCharsetDamage(raw []byte, storedSubject, storedBody string) (CharsetRecovery, bool) {
	msg, err := Parse(raw, "")
	if err != nil {
		return CharsetRecovery{}, false
	}
	recovery := CharsetRecovery{Body: storedBody}
	if subject, ok := msg.recoveredSubject(raw, storedSubject); ok {
		recovery.Subject = &subject
	}
	if body, completeness, ok := msg.recoveredCharsetBody(storedBody); ok {
		recovery.Body, recovery.BodyChanged, recovery.Completeness = body, true, completeness
	}
	return recovery, recovery.Subject != nil || recovery.BodyChanged
}

// recoveredSubject is the subject capture stores now, when stored is the one
// the parser before charset decoding stored from raw.
func (m Message) recoveredSubject(raw []byte, stored string) (string, bool) {
	root, err := readEntity(raw)
	if err != nil {
		return "", false
	}
	header := mail.Header{Header: root.Header}
	undecoded, _ := header.Subject()
	current := credentialmask.MaskText(m.subject)
	return current, stored != "" && credentialmask.MaskText(strings.TrimSpace(undecoded)) == stored && current != stored
}

// recoveredCharsetBody is the body capture stores now, when stored is that body
// with each non-ASCII character replaced by one mark, cut at either limit.
func (m Message) recoveredCharsetBody(stored string) (string, capture.BodyCompleteness, bool) {
	for _, mark := range []string{"?", "�"} {
		damaged := standInForNonASCII(m.body, mark)
		if damaged == m.body {
			return "", "", false
		}
		for _, limit := range []int{maxBodyLen, legacyMaxBodyLen} {
			if _, held := heldShape(m.storedShapes(damaged, limit), stored); held {
				body, completeness := m.storedBody()
				return body, completeness, body != stored
			}
		}
	}
	return "", "", false
}

func standInForNonASCII(text, mark string) string {
	var out strings.Builder
	for _, r := range text {
		if r > unicode.MaxASCII {
			out.WriteString(mark)
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
