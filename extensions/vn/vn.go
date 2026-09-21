// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package vn is the Vietnamese jurisdiction pack as a stable-tier extension:
// its directory under extensions/ IS the enablement. Core code never contains a
// jurisdiction string — this unit is where Vietnam lives.
//
// V2 declares the outbound-messaging rules Vietnamese law places on advertising
// email, and states THREE instruments rather than one.
//
// Decree 91/2020/ND-CP is where every obligation below comes from: the daily
// ceiling, the [QC] label, the advertiser identification, the acknowledged
// opt-out. That has not changed, and the engine applies exactly what it did at
// version 1.
//
// Law 91/2025/QH15 and Decree 356/2025/ND-CP both took effect on 1 January 2026
// and govern personal data protection. They are stated here because a
// Vietnamese recipient of advertising email is a data subject under them, so a
// decision taken from that date sits under all three — NOT because either one
// amends the 2020 decree. Whether they impose an outbound-messaging obligation
// this pack does not yet declare is an open question for a Vietnamese lawyer,
// and stating them is what makes that question askable from the record.
//
// The version moves because the instruments moved. A decision recording
// "vn version 2" can be read back against the law that was live when it was
// taken, which a version alone cannot answer.
//
// It declares NO retention class: the decree and the Vietnamese accounting law
// bind records this CRM does not hold, and a floor no record can carry would be
// documentation posing as enforcement.
package vn

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// New returns the unit's declaration (the ADR-0120 §4 constructor contract the
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

// vnInstruments are the laws this rule set rests on.
//
// THREE, and the 2020 decree is the one every obligation below rests on. The
// 2025 pair governs personal data protection and took effect on 1 January 2026;
// neither amends the decree. They are stated because a recipient of Vietnamese
// advertising email is a data subject under them, so a decision from that date
// sits under all three — and a pack listing only the decree would leave a
// record that cannot say so.
//
// Stated for the READER of a decision, never applied: nothing in the engine
// consults an instrument, and the obligations these instruments carry are the
// fields below. A pack that put a rule here instead would be declaring law the
// engine cannot apply.
func vnInstruments() []messaging.Instrument {
	// Inside the function, not at package level: a var initializer would run at
	// import, before the declaration is validated, and the composition
	// generator refuses one for exactly that reason.
	//
	// When Law 91/2025/QH15 and Decree 356/2025/ND-CP took effect — their own
	// commencement date, not the date this pack stated them.
	commencement2025 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []messaging.Instrument{
		{
			Name:          "Decree 91/2020/ND-CP",
			EffectiveFrom: time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC),
		},
		{Name: "Law 91/2025/QH15", EffectiveFrom: commencement2025},
		{Name: "Decree 356/2025/ND-CP", EffectiveFrom: commencement2025},
	}
}

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
// advertising in its subject, and the decree fixes the label. Declaring it
// while nothing applies it is deliberate: the rule is what the decree says, and
// a pack that waited would leave the obligation unrecorded until the machinery
// caught up. Which obligations here bind today is not this comment's to claim —
// TestEveryDeclaredMessagingObligationIsAppliedOrRecorded
// (backend/gates/messagingruleapplied_test.go) holds the answer, and fails when
// a field is applied or abandoned without the record moving with it.
//
// ADVERTISER IDENTIFICATION is Art. 13: an advertising message names the
// advertiser and gives a way to reach them. It is declared as an
// AdvertiserContact disclosure alongside the Art. 13 GDPR-shaped controller
// disclosures, because a Vietnamese recipient is owed BOTH — who is processing
// their data and who is advertising to them are the same organisation here and
// need not be, and the two obligations come from different instruments. It also
// needs an advertiser phone and website, which the installation settings do not
// carry.
//
// THE ACKNOWLEDGED OPT-OUT is Art. 16: a recipient who refuses further
// advertising is owed a confirmation that their refusal was received, sent
// within twenty-four hours and carrying no advertising of its own. The flag says
// one is owed; the engine's controller lane is what will send it, which is the
// only lane that may write to somebody who has just suppressed themselves.
//
// THE DAILY CEILING is the one rule here the engine applies today. It refuses
// regardless of rollout mode, because an installation declaring a country is
// asserting which law it sends under.
//
// THE WINDOWS are the core defaults, restated so this pack says what it applies
// rather than inheriting silently. Neither bounds a same-thread reply.
func messagingRules() messaging.Rules {
	return messaging.Rules{
		Jurisdiction: "vn",
		// 2, because the instruments below changed. The obligations the engine
		// applies did not: a decision recording version 1 was judged under
		// Decree 91/2020 alone, and one recording version 2 was judged under
		// the same decree as amended. The number is what tells those two apart
		// in a record read years later, which is the only reason it moves.
		Version:            2,
		Instruments:        vnInstruments(),
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
