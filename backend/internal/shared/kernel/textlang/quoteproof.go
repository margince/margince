// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textlang

import "github.com/margince/margince/backend/internal/shared/kernel/values"

// The cuts a quote proof can be made against. A proof hashes the text the cut
// leaves, so it verifies only against the same cut.
const (
	// quoteCutBeforeSecondSender is the cut of every proof stored before cuts
	// were named: a second sender header was not yet read as a quote.
	quoteCutBeforeSecondSender = 0
	// QuoteCutSecondSender reads a second sender header in the envelope's run as
	// the start of a quoted Outlook block, as every other reader does.
	QuoteCutSecondSender = 1
)

// ProveSenderQuote binds a quote to the sender's own text under today's cut.
func ProveSenderQuote(body, quote string) (values.QuoteProof, bool) {
	proof, ok := values.ProveQuote(CurrentMessage(body), quote)
	proof.Cut = QuoteCutSecondSender
	return proof, ok
}

// VerifySenderQuote checks a stored proof against the sender's own text, cut
// the way the proof was made. A proof stored before today's cut still verifies.
func VerifySenderQuote(body string, proof values.QuoteProof) (string, bool) {
	if proof.Cut == quoteCutBeforeSecondSender {
		return values.VerifyQuote(currentMessageTo(body, provenQuoteStart), proof)
	}
	return values.VerifyQuote(CurrentMessage(body), proof)
}
