// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The one thing this account needs today.
//
// The contact page has carried this card for a while; the company page had
// nothing, so an account could owe three promises and open on a screen that
// said nothing about any of them. A reader who works accounts rather than
// contacts had to open each contact to find out.
//
// WHAT IT FIRES ON. What we OWE the account's contacts, from both places a
// promise gets written down: a task somebody filed, and a commitment an
// extractor read out of a conversation. Which of the two it came from is a
// fact about this system rather than about what is owed, so both rank by one
// rule — kernel/owedwork, the same package the contact page's card reads. Two
// screens answering "what do we owe them?" differently was the defect this
// closes.
//
// ONE CARD, NOT A LADDER. The contact page walks eight rungs; this fires on
// one thing and says "nothing needed" otherwise. That is deliberate: a rung
// earns its place by being actionable on THIS record, and the account-level
// answers to "they went quiet" or "a role changed" are the contact page's to
// give. When a second account-level reason appears, it joins here in a fixed
// order the way the contact ladder is ordered.

import (
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sectionMoments names this section in sections_omitted, so a caller told the
// card is absent knows it was withheld rather than empty.
const sectionMoments = crmcontracts.Company360SectionsOmitted("moments")

// momentRuleVersion stamps which spelling of the rule produced a card, so a
// stored dismissal can be told apart from one made against an older reading.
const momentRuleVersion = "company-moment-owed-v1"

// readMoment selects the card and hangs it on the page.
//
// It runs after the sections it reads from, the way the contact page's does:
// the next-steps section has already gathered the account's open tasks under
// the caller's own grants, so re-reading them here would be a second answer to
// a question already answered — and one that could disagree with the list the
// reader is looking at.
func (a *assembly) readMoment() error {
	if err := auth.Require(a.ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	claims, complete, err := a.svc.contacts.OpenCommitmentsForCompany(
		a.ctx, a.tx, a.companyID.UUID, companyMomentScanCap)
	if err != nil {
		return err
	}
	if !complete {
		// A promise dropped for row scope would leave the card silent, and a
		// silent card reads as "this account owes nothing" — the one thing it
		// must never say by accident. The page already carries this flag for
		// the attention facts; a reader who sees it knows the card is speaking
		// about less than the whole account.
		markAttentionWithheld(a.out)
	}
	// The card's OWN task read, not the section's rows. The section shows the
	// twenty-five earliest deadlines; this ranks over the set and asks which
	// promise slipped most recently, and on an account owing more than a page
	// of tasks the row that answers it is exactly the one the page dropped.
	tasks, filed, err := openTaskPromises(a.ctx, a.tx, a.companyID, a.now, a.opts, companyMomentScanCap)
	if err != nil {
		return err
	}
	moment := accountMoment(a.now, tasks, filed, claims)
	momentaction.Withhold(a.ctx, &moment)
	a.out.Moment = &moment
	return nil
}

// markAttentionWithheld says the page is speaking about less than the account.
//
// A promise the caller may not see is dropped by the read's own scope clauses,
// so the card would otherwise show whatever remains — or the quiet state — and
// a quiet card reads as "this account owes nothing". That is the one thing it
// must never say by accident, so the omission is stated rather than absorbed.
func markAttentionWithheld(out *crmcontracts.Company360) {
	withheld := true
	out.AttentionWithheld = &withheld
}

// companyMomentScanCap bounds the promise read behind the card. Wide, because the
// card RANKS over the set rather than showing it: a bound that decided which
// promise is most urgent would be the read stopping early wearing the rule's
// clothes.
const companyMomentScanCap = 200

// accountMoment is the card itself: the promise that most needs answering, or
// the quiet state when there is none.
func accountMoment(now time.Time, tasks []crmcontracts.Company360NextStep, filed []time.Time, claims []contacts.CompanyCommitment) crmcontracts.ContactMoment {
	items := accountPromises(tasks, filed, claims)
	if slipped, ok := owedwork.MostRecentlySlipped(items, now); ok {
		return promiseCard(now, slipped, true)
	}
	if next, ok := owedwork.Soonest(items, now); ok {
		return promiseCard(now, next, false)
	}
	return accountNothingNeeded()
}

// accountPromises gathers the account's open promises, from both sources, in
// the shape the shared ranking reads.
//
// Held by: TestTheAccountCardRanksBothSourcesByDateAlone (moment_test.go),
// which fails if either source stops reaching the card.
//
// A UNION, NOT A JOIN. Nothing writes conversation_claim.task_activity_id, so
// an extracted commitment and a task about the same thing are two unlinked
// rows. A promise recorded both ways may appear as two, which is the honest
// answer until that link is written.
func accountPromises(tasks []crmcontracts.Company360NextStep, filed []time.Time, claims []contacts.CompanyCommitment) []owedwork.Item {
	var items []owedwork.Item
	for i, step := range tasks {
		item := owedwork.Item{Ref: step, Source: owedwork.FromTask, DueAt: step.DueAt}
		// The filing moment breaks ties between two promises sharing a due
		// date, and the contact page ranks on it too. Leaving it zero here
		// would make the same two promises rank differently on the two pages.
		if i < len(filed) {
			item.FiledAt = filed[i]
		}
		items = append(items, item)
	}
	for _, claim := range claims {
		items = append(items, owedwork.Item{
			Ref: claim, Source: owedwork.FromClaim,
			DueAt: claim.DueAt, FiledAt: claim.OccurredAt,
		})
	}
	return items
}

// promiseCard renders one promise, whichever source it came from.
//
// The two sources differ in what they can show, not in what they mean: a claim
// quotes the sentence the promise was made in, a task carries only what
// somebody retyped. Both name who it is owed to, because on an account page
// "we owe something" is not actionable until a reader knows to whom.
func promiseCard(now time.Time, item owedwork.Item, late bool) crmcontracts.ContactMoment {
	switch promise := item.Ref.(type) {
	case contacts.CompanyCommitment:
		return accountClaimCard(now, promise, late)
	case crmcontracts.Company360NextStep:
		return accountTaskCard(now, promise, late)
	default:
		// accountPromises puts only those two types in a Ref; a third would be
		// a promise this card cannot render rather than one to show blank.
		return accountNothingNeeded()
	}
}

// accountClaimCard is the card for a promise read out of a conversation.
func accountClaimCard(now time.Time, claim contacts.CompanyCommitment, late bool) crmcontracts.ContactMoment {
	activity := openapi_types.UUID(claim.ActivityID)
	observed := claim.OccurredAt
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:       crmcontracts.ContactMomentEvidenceTypeActivity,
		Id:         &activity,
		Label:      claim.Body,
		Snippet:    &claim.SourceQuote,
		ObservedAt: &observed,
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            momentKey("moment:account_promise_claim", openapi_types.UUID(claim.ID)),
		Rule:                ruleFor(late),
		RuleVersion:         ptrOf(momentRuleVersion),
		EvidenceFingerprint: accountFingerprint(evidence),
		Headline:            owedHeadline(claim.ContactName, claim.Body),
		WhyNow:              owedWhyNow(now, claim.DueAt),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         &observed,
		RecommendedAction:   openTheContact(claim.ContactID),
	}
}

// accountTaskCard is the card for a promise somebody filed as a task.
func accountTaskCard(now time.Time, step crmcontracts.Company360NextStep, late bool) crmcontracts.ContactMoment {
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:  crmcontracts.ContactMomentEvidenceTypeTask,
		Id:    &step.ActivityId,
		Label: step.Subject,
	}}
	moment := crmcontracts.ContactMoment{
		ClaimKey:            momentKey("moment:account_promise_task", step.ActivityId),
		Rule:                ruleFor(late),
		RuleVersion:         ptrOf(momentRuleVersion),
		EvidenceFingerprint: accountFingerprint(evidence),
		Headline:            owedHeadline("", step.Subject),
		WhyNow:              owedWhyNow(now, step.DueAt),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		RecommendedAction:   openTheTask(step),
	}
	return moment
}

// ruleFor names the rung, which is what a client renders its tone from.
func ruleFor(late bool) crmcontracts.ContactMomentRule {
	if late {
		return crmcontracts.ContactMomentRuleOverduePromise
	}
	return crmcontracts.ContactMomentRuleOpenPromise
}

// owedHeadline names the promise and, where the row carries it, who is waiting
// for it.
//
// A claim names its contact; a task row carries a linked contact ID and no name,
// so its card says "them" rather than fetching a name the section did not read.
// A promise whose contact this caller may not name still belongs on the card —
// the account owes it either way — so the sentence drops the name, never the
// promise.
func owedHeadline(who, what string) string {
	if who == "" {
		return fmt.Sprintf("You owe them: %s", what)
	}
	return fmt.Sprintf("You owe %s: %s", who, what)
}

// owedWhyNow says what the date on the promise is, in the reader's terms.
func owedWhyNow(now time.Time, due *time.Time) string {
	if due == nil {
		return "Promised with no date set. It stays open until somebody does it or closes it."
	}
	if past, ok := deadline.DaysPast(due, now); ok {
		return fmt.Sprintf("Due %d days ago and still open.", past)
	}
	if days := elapsed.FullDaysUntil(now, *due); days > 0 {
		return fmt.Sprintf("Due in %d days.", days)
	}
	return "Due today."
}

// accountNothingNeeded is the quiet success state — an answer, not a blank
// card. A reader came to the page for an answer, and "nothing is outstanding"
// is one.
//
// IT CLAIMS ONLY WHAT THIS CARD CHECKED. The card fires on owed promises and
// on nothing else, while the same payload carries the suggestions the rules
// found — an unanswered message, a deal that has stalled, an account with
// nothing scheduled. "Nothing needs you today" beside three of those is the
// page contradicting itself, in the one element a reader takes as the verdict,
// and it reads that way to every client: the screen, and an agent reading the
// account through the tool surface. The contact page's rung 10 may say it,
// having walked eight rungs to get there; this card may not, and the headline
// says what it looked at instead.
func accountNothingNeeded() crmcontracts.ContactMoment {
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:nothing_needed",
		Rule:                crmcontracts.ContactMomentRuleNothingNeeded,
		RuleVersion:         ptrOf(momentRuleVersion),
		EvidenceFingerprint: "quiet",
		Headline:            "Nothing is owed to this account",
		WhyNow:              "No promise to this account is open or coming due.",
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            []crmcontracts.ContactMomentEvidence{},
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindLogActivity,
			Label: "Log something",
			State: crmcontracts.ContactMomentActionStateWillConfirm,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface: crmcontracts.ContactMomentDestinationSurfaceActivityLog,
			},
		},
	}
}

// momentKey names WHICH promise a card is about, not merely that it is a
// promise card.
//
// A dismissal is one row per (reader, record, claim key), so a bare rung name
// would give every promise on an account one shared dismissal: putting the
// first away would silence the second, and the reader would never be told it
// existed.
func momentKey(rung string, id openapi_types.UUID) string {
	return rung + ":" + id.String()
}

// ptrOf is the address of a value the contract wants as a pointer.
func ptrOf[T any](v T) *T { return &v }

// accountFingerprint digests what this card fired on, through the same hash
// the contact page's cards use.
func accountFingerprint(evidence []crmcontracts.ContactMomentEvidence) string {
	marks := make([]owedwork.Mark, 0, len(evidence))
	for _, e := range evidence {
		mark := owedwork.Mark{Kind: string(e.Type), At: e.ObservedAt}
		if e.Id != nil {
			mark.ID = e.Id.String()
		}
		marks = append(marks, mark)
	}
	return owedwork.Fingerprint(marks)
}

// openTheContact sends the reader to the record the promise lives on, which is
// where they can see the conversation it was made in and act on it.
func openTheContact(contactID ids.ContactID) crmcontracts.ContactMomentAction {
	id := openapi_types.UUID(contactID.UUID)
	return crmcontracts.ContactMomentAction{
		Kind:  crmcontracts.ContactMomentActionKindOpenRecord,
		Label: "Open the contact",
		State: crmcontracts.ContactMomentActionStateAvailable,
		Destination: &crmcontracts.ContactMomentDestination{
			Surface:    crmcontracts.ContactMomentDestinationSurfaceRecord,
			EntityType: entityTypeOf(crmcontracts.ContactMomentDestinationEntityTypeContact),
			EntityId:   &id,
		},
	}
}

// openTheTask sends the reader to the record the task is filed against, or
// says it cannot when the task names none.
func openTheTask(step crmcontracts.Company360NextStep) crmcontracts.ContactMomentAction {
	if step.LinkedContactId != nil {
		return openTheContact(ids.From[ids.ContactKind](ids.UUID(*step.LinkedContactId)))
	}
	if step.LinkedDealId != nil {
		return crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindOpenRecord,
			Label: "Open the deal",
			State: crmcontracts.ContactMomentActionStateAvailable,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface:    crmcontracts.ContactMomentDestinationSurfaceRecord,
				EntityType: entityTypeOf(crmcontracts.ContactMomentDestinationEntityTypeDeal),
				EntityId:   step.LinkedDealId,
			},
		}
	}
	// The task is the account's own and names no record beneath it. The card
	// still states the promise; it offers no button rather than one that would
	// land the reader nowhere.
	return crmcontracts.ContactMomentAction{
		Kind:  crmcontracts.ContactMomentActionKindCompleteTask,
		Label: "Open it from the task list",
		State: crmcontracts.ContactMomentActionStateBlocked,
	}
}

func entityTypeOf(v crmcontracts.ContactMomentDestinationEntityType) *crmcontracts.ContactMomentDestinationEntityType {
	return &v
}
