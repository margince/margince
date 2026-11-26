// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// EnglishWords renders the subject and the body in English, for the plain text
// a record stores beside its descriptor.
func (m *Message) EnglishWords() (subject, body string, err error) {
	if subject, err = m.Subject.Render(textlang.English); err != nil {
		return "", "", err
	}
	if body, err = m.Body.Render(textlang.English); err != nil {
		return "", "", err
	}
	return subject, body, nil
}

// SubjectIn is the subject a reader sees. While the descriptor still matches
// the stored text, it is rendered in their language; else it is shown as
// written. A read that skips this names one task in two languages.
func (m *Message) SubjectIn(subject string, lang textlang.Lang) string {
	if m == nil || m.Version != 1 {
		return subject
	}
	if shown := m.Subject.Display(subject, lang); shown != nil {
		return *shown
	}
	return subject
}

// BodyIn is SubjectIn for the body.
func (m *Message) BodyIn(body string, lang textlang.Lang) string {
	if m == nil || m.Version != 1 {
		return body
	}
	if shown := m.Body.Display(body, lang); shown != nil {
		return *shown
	}
	return body
}
