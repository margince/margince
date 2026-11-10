// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// How long a piece of evidence keeps supporting a message.
//
// Every window here bounds an UNPROMPTED follow-up and none of them bounds a
// same-thread reply. The subject wrote to us and did not withdraw, so a rep
// answering a months-old thread is doing the ordinary thing — a window that
// refused that would be restricting correspondence rather than advertising,
// which is the failure mode that makes reps route around the product.
//
// A jurisdiction pack may shorten any of these; the packs that ship declare
// exactly these numbers so a reader can see they agree rather than inheriting
// silently. What a pack must not do is LENGTHEN one: the fold takes the shorter
// of the two (messagingrules.shorterWindow), so a pack naming a longer window
// than the core default is simply ignored.

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension/messaging"
)

const (
	// defaultReplyWindow is how long an inbound message keeps making an
	// UNPROMPTED follow-up lawful when there is no thread to continue.
	//
	// Twelve months. Hours because the type is a Duration and a month is not
	// one; the comparison is against a recorded timestamp, so the small drift
	// against calendar months falls on the permissive side of a bound that
	// governs a follow-up rather than a refusal.
	defaultReplyWindow = 365 * 24 * time.Hour

	// defaultDealFollowUpWindow is how long a live opportunity keeps
	// supporting an unprompted follow-up after the last interaction. Six
	// months, on the same reading as above.
	defaultDealFollowUpWindow = 182 * 24 * time.Hour
)

// packRules is what the recipient's jurisdiction sets: the spans that bind a
// follow-up, and the marketing exception it grants.
//
// One struct because it is one read. packRulesFor is the single resolver on the
// send path, and a second would be two places that can disagree about which
// pack is in force.
type packRules struct {
	reply      time.Duration
	dealFollow time.Duration
	// marketingException is the exception this jurisdiction allows. It is nil
	// for a pack that declares none and for the baseline.
	marketingException *messaging.MarketingException
}

// packRulesFor answers the rules that bind a message to a recipient in this
// country, the marketing selector's answer.
//
// A jurisdiction that declares a window shortens the core default. One that
// declares none, and a recipient whose country is unknown or has no pack, gets
// the defaults above. So requested correspondence flows when nobody has filled
// in an address.
func packRulesFor(country RecipientCountry) packRules {
	out := packRules{reply: defaultReplyWindow, dealFollow: defaultDealFollowUpWindow}
	rules, _, applicable := applicableRules(country.Code)
	if !applicable {
		return out
	}
	// Zero means the pack says nothing about this window, which is not the same
	// as a pack asking for zero — Rules.Validate refuses a negative one and the
	// published contract documents zero as "the core default".
	if rules.ReplyWindow > 0 && rules.ReplyWindow < out.reply {
		out.reply = rules.ReplyWindow
	}
	if rules.DealFollowUpWindow > 0 && rules.DealFollowUpWindow < out.dealFollow {
		out.dealFollow = rules.DealFollowUpWindow
	}
	// THE EXCEPTION IS THE PACK'S TO DECLARE, and a jurisdiction that declares
	// none grants none. A declared one still grants nothing today: the verdict
	// refuses it and names it in the reason.
	for i, e := range rules.MarketingExceptions {
		if e.Kind == messaging.ExistingCustomer {
			out.marketingException = &rules.MarketingExceptions[i]
			break
		}
	}
	return out
}
