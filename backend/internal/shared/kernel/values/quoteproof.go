// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// QuoteProof locates words without retaining another copy of source content.
type QuoteProof struct {
	Start  int    `json:"start"`
	End    int    `json:"end"`
	SHA256 string `json:"sha256"`
	// Cut names how the source text was cut before it was hashed. Zero is the
	// cut a proof stored before cuts were named was made with.
	Cut int `json:"cut,omitempty"`
}

// ProveQuote binds a quote to normalized source content without retaining its words.
func ProveQuote(text, quote string) (QuoteProof, bool) {
	text, quote = CollapseSpace(text), CollapseSpace(quote)
	start := strings.Index(text, quote)
	if quote == "" || start < 0 {
		return QuoteProof{}, false
	}
	return QuoteProof{Start: start, End: start + len(quote), SHA256: quoteDigest(text)}, true
}

// VerifyQuote reconstructs matching words; the caller must authorize the source first.
func VerifyQuote(text string, proof QuoteProof) (string, bool) {
	text = CollapseSpace(text)
	if proof.Start < 0 || proof.End <= proof.Start || proof.End > len(text) {
		return "", false
	}
	quote := text[proof.Start:proof.End]
	if quoteDigest(text) != proof.SHA256 {
		return "", false
	}
	return quote, true
}

func quoteDigest(quote string) string {
	digest := sha256.Sum256([]byte(quote))
	return hex.EncodeToString(digest[:])
}
