// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/platform/htmltext"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// offerWords are words that make an offer, matched as whole words. The list
// stays narrow because a word here turns an ordinary reply into a marketing review.
var offerWords = []string{
	"discount", "special offer", "limited offer", "exclusive offer", "promo code", "promotion",
	"free trial", "price list", "pricing", "our offer", "price quote", "quotation",
	"we can offer", "we offer", "offer you",
	"kostenlos testen", "Kostenvoranschlag", "wir bieten Ihnen",
	"khuyến mãi", "giảm giá", "ưu đãi", "báo giá", "miễn phí", "chiết khấu", "chúng tôi cung cấp",
}

// offerStems are German offer words matched inside any word, because German
// builds compounds on them ("Sonderrabatt", "Angebotspreis").
var offerStems = []string{"angebot", "rabatt", "gutschein", "preisliste"}

// offerAmount is a price, with the currency before or after the number, or a
// percentage off. A currency sign ends a word on its own, so word edges are
// spelled out rather than taken from \b.
var offerAmount = regexp.MustCompile(
	`€\s?\d|\$\s?\d|(^|[^\p{L}\p{N}])(eur|usd|vnd)\s?\d|\d[\d.,]*\s?(€|eur|usd|vnd|đ|\$)([^\p{L}\p{N}]|$)|\d+\s?%\s?(off|rabatt|giảm)([^\p{L}\p{N}]|$)`)

// invisible drops the characters that render as nothing, so "Ange\u00adbot"
// still reads as the word a recipient sees.
var invisible = strings.NewReplacer("\u00ad", "", "\u200b", "", "\u200c", "", "\u200d", "", "\u2060", "", "\ufeff", "")

// authoredOffer reports whether the words a sender wrote, or the names of the
// files they attached, make an offer. A quoted line counts too. The composer
// keeps the thread out of the authored body, so a quote marker is typed.
func authoredOffer(subject, body string, fileNames []string) bool {
	text := invisible.Replace(norm.NFC.String(strings.Join(append([]string{subject, body}, fileNames...), "\n")))
	lowered := strings.ToLower(text)
	return len(claimsIn(text, offerWords)) > 0 || offerAmount.MatchString(lowered) ||
		slices.ContainsFunc(offerStems, func(stem string) bool { return strings.Contains(lowered, stem) })
}

// withAuthoredOffer reads the authored words into the evidence. The engine
// reads them itself, so no caller can vouch that a message makes no offer.
func withAuthoredOffer(req commsauthz.Request) commsauthz.Request {
	body := req.AuthoredBody
	if req.AuthoredHTML != "" {
		visible, cut := htmltext.Render(req.AuthoredHTML)
		if cut {
			// Markup past the render cap was never read, so it is judged as
			// though it made an offer.
			req.Evidence.AuthoredOffer = true
			return req
		}
		body += "\n" + visible
	}
	req.Evidence.AuthoredOffer = authoredOffer(req.AuthoredSubject, body, req.AuthoredFileNames)
	return req
}
