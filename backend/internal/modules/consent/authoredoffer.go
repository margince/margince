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

// offerWords are words that make an offer. The list stays narrow because a word
// here turns an ordinary reply into a marketing review.
var offerWords = []string{
	"discount", "special offer", "limited offer", "exclusive offer", "promo code", "promotion",
	"free trial", "price list", "pricing", "our offer", "quote", "quotation",
	"Rabatt", "Sonderangebot", "Angebot", "Aktion", "Gutschein", "Preisliste", "kostenlos testen",
	"Kostenvoranschlag",
	"khuyến mãi", "giảm giá", "ưu đãi", "báo giá", "miễn phí", "chiết khấu",
}

// offerAmount is a price or a percentage off. A currency sign ends a word on
// its own, so the edge after it is spelled out rather than taken from \b.
var offerAmount = regexp.MustCompile(
	`€\s?\d|\$\s?\d|\d[\d.,]*\s?(€|eur|usd|vnd|đ|\$)([^\p{L}\p{N}]|$)|\d+\s?%\s?(off|rabatt|giảm)([^\p{L}\p{N}]|$)`)

// authoredOffer reports whether the words a sender wrote make an offer. Quoted
// lines are someone else's words, so an offer the sender only quotes is not one.
func authoredOffer(subject, body string) bool {
	lines := slices.DeleteFunc(strings.Split(norm.NFC.String(subject+"\n"+body), "\n"), func(line string) bool {
		return strings.HasPrefix(strings.TrimSpace(line), ">")
	})
	text := strings.Join(lines, "\n")
	return len(claimsIn(text, offerWords)) > 0 || offerAmount.MatchString(strings.ToLower(text))
}

// withAuthoredOffer reads the authored words into the evidence. The engine
// reads them itself, so no caller can vouch that a message makes no offer.
func withAuthoredOffer(req commsauthz.Request) commsauthz.Request {
	req.Evidence.AuthoredOffer = authoredOffer(req.AuthoredSubject, req.AuthoredBody)
	return req
}
