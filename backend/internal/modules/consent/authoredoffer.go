// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// offerWords are words that make an offer, matched as whole words. The list
// stays narrow because a word here turns an ordinary reply into a marketing review.
var offerWords = []string{
	"discount", "special offer", "limited offer", "exclusive offer", "promo code", "promotion",
	"free trial", "price list", "pricing", "our offer", "quote", "quotation",
	"kostenlos testen", "Kostenvoranschlag",
	"khuyến mãi", "giảm giá", "ưu đãi", "báo giá", "miễn phí", "chiết khấu",
}

// offerStems are German offer words matched inside any word, because German
// builds compounds on them ("Sonderrabatt", "Angebotspreis").
var offerStems = []string{"angebot", "rabatt", "gutschein", "preisliste"}

// offerAmount is a price or a percentage off. A currency sign ends a word on
// its own, so the edge after it is spelled out rather than taken from \b.
var offerAmount = regexp.MustCompile(
	`€\s?\d|\$\s?\d|\d[\d.,]*\s?(€|eur|usd|vnd|đ|\$)([^\p{L}\p{N}]|$)|\d+\s?%\s?(off|rabatt|giảm)([^\p{L}\p{N}]|$)`)

// authoredOffer reports whether the words a sender wrote, or the names of the
// files they attached, make an offer. Every line counts, a quoted one too: the
// composer keeps the thread out of the authored body, so a quote marker there
// is the sender's own typing.
func authoredOffer(subject, body string, fileNames []string) bool {
	text := norm.NFC.String(strings.Join(append([]string{subject, body}, fileNames...), "\n"))
	lowered := strings.ToLower(text)
	return len(claimsIn(text, offerWords)) > 0 || offerAmount.MatchString(lowered) ||
		slices.ContainsFunc(offerStems, func(stem string) bool { return strings.Contains(lowered, stem) })
}

// withAuthoredOffer reads the authored words into the evidence. The engine
// reads them itself, so no caller can vouch that a message makes no offer.
func withAuthoredOffer(req commsauthz.Request) commsauthz.Request {
	req.Evidence.AuthoredOffer = authoredOffer(req.AuthoredSubject, req.AuthoredBody, req.AuthoredFileNames)
	return req
}
