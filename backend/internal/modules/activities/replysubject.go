// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"unicode"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ReplySubject keeps an email on its selected subject, including a follow-up to sent mail.
// The browser prefills the same subject; shared fixture cases hold both renderings.
// Inbound authority remains IsMailThread's separate question. The drafting
// floor may suggest a subject; this final email header supersedes its prefix.
// Only Re: is normalized: a forward marker is part of the selected subject.
func ReplySubject(kind crmcontracts.ActivityKind, topic, fallback string) string {
	if kind != crmcontracts.ActivityKindEmail {
		return fallback
	}
	if strings.TrimFunc(topic, subjectSpace) == "" {
		topic = fallback
	}
	topic = strings.TrimFunc(topic, subjectSpace)
	for len(topic) >= 3 && strings.EqualFold(topic[:3], "re:") {
		topic = strings.TrimLeftFunc(topic[3:], subjectSpace)
	}
	return "Re: " + topic
}

// IsReplySubjectOf reports whether subject is topic as a reply carries it, with
// or without Re:. The composer prefills that subject, so it is the thread's
// history and not words the rep wrote.
func IsReplySubjectOf(subject, topic string) bool {
	email := crmcontracts.ActivityKindEmail
	return strings.EqualFold(ReplySubject(email, subject, ""), ReplySubject(email, topic, ""))
}

// ECMAScript also treats the byte-order mark as whitespace.
func subjectSpace(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' }
