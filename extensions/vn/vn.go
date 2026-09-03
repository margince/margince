// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package vn is the Vietnamese jurisdiction pack as a stable-tier extension:
// its directory under extensions/ IS the enablement. Core code never contains a
// jurisdiction string — this unit is where Vietnam lives.
//
// V1 declares the outbound-messaging rules Decree 91/2020/ND-CP places on
// advertising email. It declares NO retention class: the decree and the
// Vietnamese accounting law bind records this CRM does not hold, and a floor no
// record can carry would be documentation posing as enforcement.
package vn

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// New returns the unit's declaration (the ADR-0069 §4 constructor contract the
// generated composition calls).
func New() extension.Extension {
	return extension.Extension{
		Name:          "vn",
		Version:       "1.0.0",
		Description:   "Vietnamese jurisdiction pack: the Decree 91/2020/ND-CP advertising-email rules — prior consent, the subject label, advertiser identity, the daily ceiling and the acknowledged opt-out.",
		Jurisdictions: []jurisdiction.Pack{pack{}},
		Messaging:     []messaging.Rules{messagingRules()},
	}
}

type pack struct{}

func (pack) Code() jurisdiction.Code { return "vn" }

// Retention: none. The core engine reads a pack's classes as statutory FLOORS
// on records the product holds, and Vietnam's record-keeping duties fall on
// accounting books and invoices — neither of which lives in a CRM. Declaring a
// class the product cannot carry would put a floor in the composition that
// nothing is ever measured against, which reads as coverage and is not.
func (pack) Retention() jurisdiction.Retention { return retention{} }

type retention struct{}

func (retention) Classes() []jurisdiction.RetentionClass { return nil }

// vnAdvertisingCap is the ceiling Decree 91/2020/ND-CP Art. 22(2) places on
// advertising email: at most three messages to one address in twenty-four
// hours, unless the recipient has agreed otherwise.
//
// The engine counts messages the recipient actually RECEIVED. That is stated on
// the FrequencyCap type and held by the engine, not by this pack — a pack says
// what the bound is, never how it is counted.
const (
	vnAdvertisingMessagesPerDay = 3
	vnAdvertisingWindow         = 24 * time.Hour
)

// advertisingLabel is the subject marking Decree 91/2020/ND-CP Art. 12 fixes
// for an advertising message. The literal lives HERE rather than on the
// published messaging surface for the reason every country string does: core
// applies a prefix a pack supplies and knows nothing about which one, so a
// second jurisdiction with a different label needs no core change.
const advertisingLabel = "[QC]"

// messagingRules is what Vietnamese law requires of an outbound message, stated
// as data the core engine applies. Nothing here decides a send.
//
// NO MARKETING EXCEPTION. Decree 91/2020/ND-CP Art. 10 permits advertising email
// only with the recipient's prior consent, and it grants no sale-derived route:
// there is no Vietnamese analogue of the German existing-customer exception. The
// empty MarketingExceptions slice is the whole rule, and it is empty
// deliberately — a reader comparing this pack against extensions/de can see that
// Vietnam has no such route rather than wondering whether one was forgotten.
// The cross-jurisdiction consequence is that evidence of a German sale
// authorizes nothing here.
//
// THE [QC] PREFIX is Art. 12: an advertising message must be labelled as
// advertising in its subject, and the decree fixes the label. The engine applies
// it exactly once — a message that already carries it is not labelled twice —
// and applies it to advertising only. An operational message wearing an
// advertising label would misdescribe itself to the recipient in the other
// direction.
//
// ADVERTISER IDENTIFICATION is Art. 13: an advertising message names the
// advertiser and gives a way to reach them. It is declared as an
// AdvertiserContact disclosure alongside the Art. 13 GDPR-shaped controller
// disclosures, because a Vietnamese recipient is owed BOTH — who is processing
// their data and who is advertising to them are the same organisation here and
// need not be, and the two obligations come from different instruments.
//
// THE ACKNOWLEDGED OPT-OUT is Art. 16: a recipient who refuses further
// advertising is owed a confirmation that their refusal was received, sent
// within twenty-four hours and carrying no advertising of its own. The flag says
// one is owed; the engine's controller lane is what sends it, which is the only
// lane that may write to somebody who has just suppressed themselves.
//
// THE WINDOWS are the core defaults, restated so this pack says what it applies
// rather than inheriting silently. Neither bounds a same-thread reply.
func messagingRules() messaging.Rules {
	return messaging.Rules{
		Jurisdiction:       "vn",
		Version:            1,
		ReplyWindow:        365 * 24 * time.Hour,
		DealFollowUpWindow: 182 * 24 * time.Hour,
		// Empty on purpose: prior consent is the only route. See above.
		MarketingExceptions: nil,
		Disclosures: []messaging.Disclosure{
			{Kind: messaging.ControllerIdentity},
			{Kind: messaging.PrivacyContact},
			{Kind: messaging.ObjectionRoute, MarketingOnly: true},
			{Kind: messaging.AdvertiserContact, MarketingOnly: true},
		},
		SubjectPrefix: advertisingLabel,
		FrequencyCap: &messaging.FrequencyCap{
			Messages: vnAdvertisingMessagesPerDay,
			Window:   vnAdvertisingWindow,
		},
		OptOutAcknowledgement: true,
	}
}
