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
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
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
	claims, complete, err := a.ourCommitments()
	if err != nil {
		return err
	}
	items, err := a.svc.promisesFrom(a.ctx, a.tx, a.companyID, a.now, a.opts, claims)
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
	// A promise this reader set aside is passed over, so the next one down
	// still reaches the card.
	items, err = a.svc.withoutSetAsidePromises(a.ctx, a.tx, a.companyID, a.now, items)
	if err != nil {
		return err
	}
	said := momentCopyFor(string(a.recordLanguage()))
	moment, err := a.svc.askDone(a.ctx, a.tx, a.companyID, a.now, said, items, said.momentOf(a.now, items))
	if err != nil {
		return err
	}
	momentaction.Withhold(a.ctx, string(said.Lang()), &moment)
	a.out.Moment = &moment
	return nil
}

// openPromises reads the promises the account's card ranks over, from both
// sources, under the caller's own grants.
//
// The card reads its own tasks rather than the next-steps section's rows. The
// section shows the twenty-five earliest deadlines, while the card asks which
// promise slipped most recently. On an account owing more than a page of
// tasks, the row that answers that is the one the page dropped.
func (s *Service) openPromises(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, opts AssembleOptions,
) ([]owedwork.Item, bool, error) {
	claims, complete, err := s.ourClaims(ctx, tx, companyID)
	if err != nil {
		return nil, false, err
	}
	items, err := s.promisesFrom(ctx, tx, companyID, now, opts, claims)
	return items, complete, err
}

// promisesFrom merges claims already read with the account's open tasks. A
// caller that holds the claims does not read them a second time.
func (s *Service) promisesFrom(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, opts AssembleOptions,
	claims []contacts.CompanyCommitment,
) ([]owedwork.Item, error) {
	tasks, filed, err := openTaskPromises(ctx, tx, companyID, now, opts, companyMomentScanCap)
	if err != nil {
		return nil, err
	}
	return accountPromises(tasks, filed, claims), nil
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
func (said momentCopy) accountMoment(now time.Time, tasks []crmcontracts.Company360NextStep, filed []time.Time, claims []contacts.CompanyCommitment) crmcontracts.ContactMoment {
	return said.momentOf(now, accountPromises(tasks, filed, claims))
}

// momentOf ranks the promises still in play and draws the card for the winner.
func (said momentCopy) momentOf(now time.Time, items []owedwork.Item) crmcontracts.ContactMoment {
	if slipped, ok := owedwork.MostRecentlySlipped(items, now); ok {
		return said.promiseCard(now, slipped, true)
	}
	if next, ok := owedwork.Soonest(items, now); ok {
		return said.promiseCard(now, next, false)
	}
	return said.accountNothingNeeded()
}

// accountPromises gathers the account's open promises, from both sources, in
// the shape the shared ranking reads.
//
// Held by: TestTheAccountCardRanksBothSourcesByDateAlone (moment_test.go),
// which fails if either source stops reaching the card.
//
// The claims arrive without the ones that became tasks
// (contacts.ourPromiseNotYetATask), so a promise is counted once. A task a rep
// typed beside an extracted promise stays a second row: nothing links them.
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
func (said momentCopy) promiseCard(now time.Time, item owedwork.Item, late bool) crmcontracts.ContactMoment {
	switch promise := item.Ref.(type) {
	case contacts.CompanyCommitment:
		return said.accountClaimCard(now, promise, late)
	case crmcontracts.Company360NextStep:
		return said.accountTaskCard(now, promise, late)
	default:
		// accountPromises puts only those two types in a Ref; a third would be
		// a promise this card cannot render rather than one to show blank.
		return said.accountNothingNeeded()
	}
}

// accountClaimCard is the card for a promise read out of a conversation.
func (said momentCopy) accountClaimCard(now time.Time, claim contacts.CompanyCommitment, late bool) crmcontracts.ContactMoment {
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
		RuleVersion:         new(momentRuleVersion),
		EvidenceFingerprint: accountFingerprint(evidence, owedwork.Mark{Kind: "claim", ID: claim.ID.String(), At: claim.DueAt}),
		Headline:            said.owedHeadline(claim.ContactName, claim.Body),
		WhyNow:              said.owedWhyNow(now, claim.DueAt),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         &observed,
		RecommendedAction:   said.openTheContact(claim.ContactID),
	}
}

// accountTaskCard is the card for a promise somebody filed as a task.
func (said momentCopy) accountTaskCard(now time.Time, step crmcontracts.Company360NextStep, late bool) crmcontracts.ContactMoment {
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:  crmcontracts.ContactMomentEvidenceTypeTask,
		Id:    &step.ActivityId,
		Label: step.Subject,
	}}
	whyNow := said.owedWhyNow(now, step.DueAt)
	if step.DueAt == nil {
		whyNow = said.Say(momentWords.TaskUndated)
	}
	moment := crmcontracts.ContactMoment{
		ClaimKey:            momentKey("moment:account_promise_task", step.ActivityId),
		Rule:                ruleFor(late),
		RuleVersion:         new(momentRuleVersion),
		EvidenceFingerprint: accountFingerprint(evidence, owedwork.Mark{Kind: "due", At: step.DueAt}),
		Headline:            said.owedHeadline("", step.Subject),
		WhyNow:              whyNow,
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		RecommendedAction:   said.openTheTask(step),
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
func (said momentCopy) owedHeadline(who, what string) string {
	if who == "" {
		return fmt.Sprintf(said.Say(momentWords.OweThem), what)
	}
	return fmt.Sprintf(said.Say(momentWords.OweNamed), who, what)
}

// owedWhyNow says what the date on the promise is, in the reader's terms.
func (said momentCopy) owedWhyNow(now time.Time, due *time.Time) string {
	if due == nil {
		return said.Say(momentWords.PromiseUndated)
	}
	return owedwork.DeadlineWords(*due, now, string(said.Lang()))
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
func (said momentCopy) accountNothingNeeded() crmcontracts.ContactMoment {
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:nothing_needed",
		Rule:                crmcontracts.ContactMomentRuleNothingNeeded,
		RuleVersion:         new(momentRuleVersion),
		EvidenceFingerprint: "quiet",
		Headline:            said.Say(momentWords.NothingOwed),
		WhyNow:              said.Say(momentWords.NothingDue),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            []crmcontracts.ContactMomentEvidence{},
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindLogActivity,
			Label: said.Say(momentWords.Log),
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

// accountFingerprint digests what this card fired on, through the same hash
// the contact page's cards use. Each card adds the promise itself. A claim adds
// its own id, since one message can hold two promises. Both add the due date,
// since a promise moved to today must return from a set-aside made earlier.
func accountFingerprint(evidence []crmcontracts.ContactMomentEvidence, promise owedwork.Mark) string {
	marks := make([]owedwork.Mark, 0, len(evidence))
	for _, e := range evidence {
		mark := owedwork.Mark{Kind: string(e.Type), At: e.ObservedAt}
		if e.Id != nil {
			mark.ID = e.Id.String()
		}
		marks = append(marks, mark)
	}
	return owedwork.Fingerprint(append(marks, promise))
}

// openTheContact sends the reader to the record the promise lives on, which is
// where they can see the conversation it was made in and act on it.
func (said momentCopy) openTheContact(contactID ids.ContactID) crmcontracts.ContactMomentAction {
	id := openapi_types.UUID(contactID.UUID)
	return crmcontracts.ContactMomentAction{
		Kind:  crmcontracts.ContactMomentActionKindOpenRecord,
		Label: said.Say(momentWords.OpenContact),
		State: crmcontracts.ContactMomentActionStateAvailable,
		Destination: &crmcontracts.ContactMomentDestination{
			Surface:    crmcontracts.ContactMomentDestinationSurfaceRecord,
			EntityType: new(crmcontracts.ContactMomentDestinationEntityTypeContact),
			EntityId:   &id,
		},
	}
}

// openTheTask sends the reader to the record the task is filed against, or
// says it cannot when the task names none.
func (said momentCopy) openTheTask(step crmcontracts.Company360NextStep) crmcontracts.ContactMomentAction {
	if step.LinkedContactId != nil {
		return said.openTheContact(ids.From[ids.ContactKind](ids.UUID(*step.LinkedContactId)))
	}
	if step.LinkedDealId != nil {
		return crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindOpenRecord,
			Label: said.Say(momentWords.OpenDeal),
			State: crmcontracts.ContactMomentActionStateAvailable,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface:    crmcontracts.ContactMomentDestinationSurfaceRecord,
				EntityType: new(crmcontracts.ContactMomentDestinationEntityTypeDeal),
				EntityId:   step.LinkedDealId,
			},
		}
	}
	// The task is the account's own and names no record beneath it. The card
	// still states the promise; it offers no button rather than one that would
	// land the reader nowhere.
	return crmcontracts.ContactMomentAction{
		Kind:  crmcontracts.ContactMomentActionKindCompleteTask,
		Label: said.Say(momentWords.OpenTaskList),
		State: crmcontracts.ContactMomentActionStateBlocked,
	}
}
