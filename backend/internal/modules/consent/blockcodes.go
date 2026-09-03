// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Why a verdict blocked, in a form code can act on.
//
// Its own file because the sentence a rep reads and the fact a caller branches
// on are two different things, and keeping them together is what let an engine
// match on prose: an ordinary copy edit to an operator message then silently
// reclassified a legal fact, and three distinct blocks — an Art. 21 objection,
// an Art. 7(3) withdrawal, and a purpose class this installation has no
// transport for — were all recorded as the first one.

// The machine-readable reasons a verdict blocks. They exist so a caller can
// act on WHICH block happened without matching on the sentence a rep reads.
const (
	// BlockObjection is Art. 21: the subject objected to direct marketing.
	BlockObjection = "objection"
	// BlockWithdrawn is Art. 7(3): they took a consent back. A different legal
	// fact from an objection, with different obligations.
	BlockWithdrawn = "withdrawn"
	// BlockUnconfirmedDOI is a grant whose round trip never completed.
	BlockUnconfirmedDOI = "unconfirmed_double_opt_in"
	// BlockNoChannel is a fact about this INSTALLATION, not about the subject:
	// no transport is wired for that purpose class.
	BlockNoChannel = "no_channel_configured"
)
