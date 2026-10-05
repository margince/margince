// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The sender's sign-off on an outbound message.
//
// The signature belongs to the identity module's world, not this one, so it
// arrives through a seam compose injects — this module may not import a
// sibling. Nil is a role wired without one, and a role that cannot read a
// signature sends unsigned mail rather than refusing to send: an unsigned
// message is what the product did for its whole life until now, and a rep
// blocked from replying because a settings row could not be read would be the
// worse failure.

import (
	"context"
	"html"
	"strings"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SignatureReader answers what the SENDER signs their mail with. It is asked
// only about the authenticated caller: a send signs with its own sender's
// sign-off, and there is no call shape here that names anybody else.
type SignatureReader interface {
	SignatureFor(ctx context.Context, userID ids.UUID) (string, error)
}

// WithSignature wires the sign-off the send path appends. Compose calls this;
// the zero Store keeps sending unsigned.
func (s *Store) WithSignature(reader SignatureReader) *Store {
	clone := *s
	clone.signature = reader
	return &clone
}

// WithSignature returns handlers whose send path appends the sender's sign-off.
func (h Handlers) WithSignature(reader SignatureReader) Handlers {
	h.store = h.store.WithSignature(reader)
	return h
}

// SignOffKind says where a send's sign-off came from.
type SignOffKind string

const (
	// SignOffNone means the send appends nothing — an agent caller, or a role
	// wired without the signature seam.
	SignOffNone SignOffKind = "none"
	// SignOffSignature means the sender's own words from their settings.
	SignOffSignature SignOffKind = "signature"
	// SignOffClosing means the sender wrote no signature, so the send closes with
	// a plain greeting in the message's language above their name.
	SignOffClosing SignOffKind = "closing"
)

// SignOff is the block a send appends beneath the message, and where it came
// from. The composer shows this same value, so what a rep reads under their
// draft is what the recipient gets.
type SignOff struct {
	Text string
	Kind SignOffKind
}

// under returns the message with the sign-off beneath it.
//
// The separator is a blank line rather than the "-- " sig-dash: this product's
// own reply parser treats that dash as a signature boundary and cuts everything
// below it (textlang.NewTextOnly), so writing one here would make our own
// captured copy of the thread end at the signature we just added.
func (o SignOff) under(body string) string {
	if o.Text == "" {
		return body
	}
	return strings.TrimRight(body, "\n") + "\n\n" + o.Text
}

// signOff is what a send of this message appends: the caller's own signature,
// or, when they wrote none, a closing in the message's language with their
// name. Body and subject are asked only for that language.
//
// An agent caller signs nothing. It acts under a human's authority but it is
// not that human, and a tool-written message arriving under somebody's
// personal sign-off — or under their name below a closing — claims a hand that
// never touched it. The From header refuses the same claim (senderDisplayName).
//
// One lookup for both renderings. Two would be two chances for the plain part
// and the markup part of one message to disagree about who signed it.
func (s *Store) signOff(ctx context.Context, body, subject string) (SignOff, error) {
	if s.signature == nil {
		return SignOff{Kind: SignOffNone}, nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return SignOff{Kind: SignOffNone}, nil
	}
	sign, err := s.signature.SignatureFor(ctx, actor.UserID)
	if err != nil {
		return SignOff{}, err
	}
	if sign = strings.TrimSpace(sign); sign != "" {
		return SignOff{Text: sign, Kind: SignOffSignature}, nil
	}
	name, err := s.senderDisplayName(ctx)
	if err != nil {
		return SignOff{}, err
	}
	closing := mailcopy.For(string(s.footerLanguage(ctx, body, subject))).SignOffClosing
	if name = strings.TrimSpace(name); name != "" {
		closing += "\n" + name
	}
	return SignOff{Text: closing, Kind: SignOffClosing}, nil
}

// signedHTML is SignOff.under's markup twin: the same sign-off and the same
// unsubscribe footer, rendered as HTML.
//
// The sign-off is PLAIN TEXT, so it is escaped before it reaches a markup
// document. A member whose sign-off contains "Weiß & Konrad <Recht>" must not
// have it silently become a broken tag, and one who typed a script tag must
// not have it run in the recipient's client.
//
// An empty markup body stays empty: a message with no HTML alternative is sent
// as a single text/plain part, and manufacturing markup here would make every
// plain send multipart for no reader's benefit.
func signedHTML(htmlBody string, sign SignOff, derived sendDeliverability) string {
	if strings.TrimSpace(htmlBody) == "" {
		return ""
	}
	out := htmlBody
	if sign.Text != "" {
		out += "\n<p>" + htmlLines(sign.Text) + "</p>"
	}
	// The DISCLOSURES before the unsubscribe footer, matching the plain-text
	// order: what the law requires the message to say, then the capability it
	// offers. A markup alternative that omitted them would disclose nothing to
	// every recipient whose client prefers HTML, while the recorded text copy
	// looked compliant.
	if disclosures := htmlDisclosures(derived.disclosures); disclosures != "" {
		out += "\n" + disclosures
	}
	if footer := derived.htmlFooter(); footer != "" {
		out += "\n" + footer
	}
	return out
}

// htmlLines escapes plain text for a markup document and keeps its line breaks,
// which a signature depends on: a name, a company and a phone number written on
// three lines are three lines to the contact who wrote them.
func htmlLines(text string) string {
	return strings.ReplaceAll(html.EscapeString(text), "\n", "<br>")
}
