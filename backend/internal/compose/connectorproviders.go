// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which capture providers this deployment's connect transport speaks for, and
// which of them arrive by consent redirect.
//
// Its own file rather than a header on connectors.go because it is a
// VOCABULARY, read by the transport, by the per-provider app resolution
// (connectorapp.go) and by the refusal each of them writes. Every one of those
// grew a copy of the list at some point, and the copies are what let a provider
// be admitted by one and unknown to the next.

import "slices"

// The OAuth capture providers. Gmail and Google Calendar (gcal) share one
// Google OAuth app; graph (Outlook mail) and graphcal (Outlook calendar) share
// one Microsoft app. Within each vendor the two differ only in scope, and each
// is its own CONNECTION: one consent apiece, so a person can bring their
// calendar without their mail and disconnect either.
const (
	providerGmail    = "gmail"
	providerGcal     = "gcal"
	providerGraph    = "graph"
	providerGraphCal = "graphcal"
)

// oauthProviders are the providers carried through a consent redirect. One list
// rather than a chain of comparisons, because the chain had to be widened in
// step with the refusal MESSAGE beside it and nothing made them widen together
// — a provider admitted by the guard but missing from the message, or the
// reverse, is a refusal that names the wrong set.
var oauthProviders = []string{providerGmail, providerGcal, providerGraph, providerGraphCal}

// isOAuthProvider reports whether a provider connects by consent redirect (as
// opposed to imap, which submits a credential).
func isOAuthProvider(provider string) bool {
	return slices.Contains(oauthProviders, provider)
}

// MailProviders are the providers that connect a MAILBOX. The other two —
// gcal and graphcal — connect a calendar, and the difference is not a detail of
// presentation: a calendar has no mail history to import backward from a date,
// no posture to take towards mail it does not carry, and no signature block to
// read a contact out of. Every mail-shaped operation is refused for one, and the
// connections screen draws none of those rows against one.
//
// The whole reason this is a named set rather than a comparison at each door:
// the screen listed a calendar as a "mailbox", drew it under an envelope beside
// the member's own email address, and offered it a mail-history import that
// answered "this mailbox type cannot be backfilled". A reader took that as the
// product refusing to import their mail.
//
// Held equal to frontend/src/screens/connectorproviders.ts's MAIL_PROVIDERS by
// backend/gates/frontendmailproviders_test.go: a screen that drew a mail row
// against a calendar and a server that refused it would disagree in front of a
// user, which is how this was reported.
func MailProviders() []string { return []string{providerGmail, providerGraph, providerIMAP} }

// IsMailProvider reports whether this provider connects a mailbox.
func IsMailProvider(provider string) bool {
	return slices.Contains(MailProviders(), provider)
}

// listedProviders are what the connect screen offers, which is the mail set:
// gcal and graphcal are each created by their paired MAIL grant rather than
// picked directly, so there is nothing for the screen to ask about them on
// their own. Derived rather than listed again — the two answers were the same
// three names written twice.
var listedProviders = MailProviders()
