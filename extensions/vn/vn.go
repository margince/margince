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
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
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
		SubjectRights: []subjectrights.Rules{subjectRights()},
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

// vnAdvertisingCap is the ceiling Decree 91/2020/ND-CP Art. 13.5 places on
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

// advertisingLabel is the subject marking Decree 91/2020/ND-CP Art. 18 fixes
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

// refusalMeans are the ways Decree 91/2020/ND-CP Art. 20.2 requires a
// recipient to be able to refuse advertising email.
func refusalMeans() []messaging.ContactMeans {
	return []messaging.ContactMeans{messaging.MeansWeb, messaging.MeansEmail, messaging.MeansPhone}
}

// superlatives are the claims Law on Advertising Art. 8.11 and Circular
// 12/2026/TT-BVHTTDL Art. 3 allow only with a listed proof shown in the mail.
// Art. 3.1 reaches a claim of a leading, unique or absolute position in any
// language, so the English forms are listed too. "nhất" alone also catches the
// forms built on it ("lớn nhất", "rẻ nhất"). "the only" and not "only": a bare
// "only" limits ("only three days left") far more often than it ranks.
func superlatives() []string {
	return []string{
		"nhất", "duy nhất", "tốt nhất", "số một", "số 1", "hàng đầu",
		"best", "no. 1", "number one", "leading", "the only", "unique",
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
// The [QC] prefix is Art. 18: an advertising message must be labelled as
// advertising in its subject, and the decree fixes the label. Declaring it
// while nothing applies it is deliberate: the rule is what the decree says, and
// a pack that waited would leave the obligation unrecorded until the machinery
// caught up. Which obligations here bind today is not this comment's to claim —
// TestEveryDeclaredMessagingObligationIsAppliedOrRecorded
// (backend/gates/messagingruleapplied_test.go) holds the answer, and fails when
// a field is applied or abandoned without the record moving with it.
//
// Advertiser identification is Art. 19: an advertising message names the
// advertiser and gives a way to reach them. It is declared as an
// AdvertiserContact disclosure beside the controller disclosures GDPR Art. 13
// shapes. A Vietnamese recipient is owed both. Who processes their data and who
// advertises to them are often one organisation, and need not be. The two
// duties come from different instruments. It also needs an advertiser phone and
// website, which the installation settings do not carry.
//
// The acknowledged opt-out is Art. 20. A recipient who refuses advertising
// email is owed a receipt at once. It states when the refusal arrived and when
// sending stopped, and carries no advertising. The deadline below is an
// internal alert, not the decree's window. The controller lane sends it: the
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
		// 2 stated the 2025 instruments. 3 applied them: the data-protection
		// rules follow the recipient, no use rests on legitimate interests, and
		// the receipt is due at once in Vietnam time. 4 adds that a consent
		// mail answers the recipient's own request. 5 asks three refusal
		// means, ends advertising with the refusal section and flags claims
		// that need a proof. The number tells a decision under each set apart.
		Version:            5,
		Instruments:        vnInstruments(),
		ReplyWindow:        365 * 24 * time.Hour,
		DealFollowUpWindow: 182 * 24 * time.Hour,
		// Empty on purpose: prior consent is the only route. See above.
		MarketingExceptions: nil,
		// And no exception a sender or an admin records replaces it.
		ConsentOnly: true,
		Disclosures: []messaging.Disclosure{
			{Kind: messaging.ControllerIdentity},
			{Kind: messaging.PrivacyContact},
			{Kind: messaging.ObjectionRoute, MarketingOnly: true, Means: refusalMeans()},
			{Kind: messaging.AdvertiserContact, MarketingOnly: true},
		},
		RefusalSectionLast: true,
		ClaimsNeedProof:    superlatives(),
		// Law on Advertising Art. 39.2. The customer is the advertiser.
		ForeignAdvertiserNeedsLocalAgency: true,
		SubjectPrefix:                     advertisingLabel,
		FrequencyCap: &messaging.FrequencyCap{
			Messages: vnAdvertisingMessagesPerDay,
			Window:   vnAdvertisingWindow,
		},
		OptOutAcknowledgement: true,
		// Art. 20.3 owes the receipt at once, and it is queued with the refusal.
		// An hour is when a receipt still unsent raises an alarm.
		OptOutAcknowledgementWithin: time.Hour,
		// Art. 20.4(a): the receipt states both times, read here in Vietnam time.
		OptOutAcknowledgementUTCOffset: 7 * time.Hour,
		// Art. 11.7 keeps registrations, refusals and receipts at least one
		// year; 366 days holds across a leap year.
		OptOutRecordsKeep: 366 * 24 * time.Hour,
		// Law 91/2025/QH15 Art. 1.2(c) reaches a foreign controller that
		// processes a Vietnamese citizen's data.
		DataProtectionFollowsRecipient: true,
		DisclosureLanguage:             "vi",
		// Art. 9 and 11.1 need consent before collection, unless an Art. 19.1
		// case applies. The law has no legitimate-interest basis.
		NoLegitimateInterests: true,
		// Decree 91/2020/ND-CP Art. 11.2: a consent request the recipient did
		// not ask for is itself an unsolicited message.
		ConsentConfirmationNeedsRequest: true,
		// Art. 11.2 governs advertising, so only the advertising classes need it.
		ConsentRequestFirstFor: messaging.AdvertisingPurposeClasses(),
	}
}

// subjectRights are the Decree 356/2025/ND-CP Art. 5 deadlines. Every request
// is owed a first answer in 2 working days. It is carried out in 10 days to
// correct (Art. 5.3), 20 to erase (Art. 5.4) and 15 to restrict (Art. 5.2),
// counted in calendar days. Access keeps the core month.
func subjectRights() subjectrights.Rules {
	return subjectrights.Rules{
		Jurisdiction: "vn",
		Version:      1,
		Instruments: []messaging.Instrument{
			{Name: "Decree 356/2025/ND-CP, Art. 5", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		Deadlines: map[subjectrights.RequestKind]subjectrights.Deadline{
			subjectrights.Rectify:     {Period: jurisdiction.Period{Days: 10}},
			subjectrights.Erasure:     {Period: jurisdiction.Period{Days: 20}},
			subjectrights.Restriction: {Period: jurisdiction.Period{Days: 15}},
		},
		ReplyWithinWorkingDays: 2,
		// Working days are Vietnamese days.
		UTCOffset: 7 * time.Hour,
	}
}
