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

// listedProviders are the mail providers a person picks between on the
// connect screen: gmail, graph, and imap. gcal and graphcal are omitted,
// since each is created by its paired MAIL grant rather than picked directly,
// so there is nothing for the screen to ask about them on their own.
var listedProviders = []string{providerGmail, providerGraph, providerIMAP}
