// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/platform/htmltext"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
// reads them itself, so no caller can vouch that a message makes no offer. A
// subject that carries the anchor's over is left to quotedSubjectFor.
func withAuthoredOffer(req commsauthz.Request, anchorSubject string) commsauthz.Request {
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
	if anchorSubject == "" || mailsubject.Topic(req.AuthoredSubject) != mailsubject.Topic(anchorSubject) {
		req.Evidence.AuthoredOffer = authoredOffer(req.AuthoredSubject, body, req.AuthoredFileNames)
		return req
	}
	req.Evidence.AuthoredOffer = authoredOffer("", body, req.AuthoredFileNames)
	req.Evidence.QuotedSubjectOffer = !req.Evidence.AuthoredOffer && authoredOffer(req.AuthoredSubject, "", nil)
	return req
}

// quotedSubjectFor settles a carried-over subject for one recipient. It is
// their own words only when they wrote the anchor; for anyone else it is an
// offer the rep sends them. Staging asks about the anchor it was given and
// keeps the anchor on the row; transmit asks that anchor again, so one
// archived or no longer authenticated since makes the subject the rep's.
func quotedSubjectFor(ctx context.Context, tx pgx.Tx, r connector.Recipient, req commsauthz.Request) (commsauthz.Request, error) {
	anchor := req.Evidence.QuotedAnchorID
	if req.Evidence.QuotedSubjectOffer {
		anchor = req.AnchorActivityID
	}
	if anchor.IsZero() {
		return req, nil
	}
	contactID, found, err := resolveContact(ctx, tx, r)
	if err != nil && !errors.Is(err, apperrors.ErrConsentNotGranted) {
		return commsauthz.Request{}, err
	}
	address, channelProvider, channelUserID := recipientSubjectAddress(r)
	subject := subjectRef{Address: address, ChannelProvider: channelProvider, ChannelUserID: channelUserID, SendingEntity: req.SendingEntity}
	if found {
		subject.Kind, subject.ID = entityContact, contactID
	}
	wrote, err := subjectWroteTheAnchor(ctx, tx, anchor, subject)
	if err != nil {
		return commsauthz.Request{}, err
	}
	req.Evidence.AuthoredOffer = req.Evidence.AuthoredOffer || !wrote
	req.Evidence.QuotedSubjectOffer = false
	req.Evidence.QuotedAnchorID = ids.UUID{}
	if wrote {
		req.Evidence.QuotedAnchorID = anchor
	}
	return req, nil
}
