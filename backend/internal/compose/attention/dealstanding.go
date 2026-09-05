// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The standing pass: a deal row says how the deal is doing, beside the step
// that acts on it.
//
// nameTheStep answers WHAT TO DO. This answers WHAT YOU ARE WALKING INTO, and
// the two were split because the reader needs both and the queue carried only
// the first. A row saying "draft a reply" over a deal that has been cold for
// three months is a different instruction from the same row over a deal closing
// on Friday, and the row did not say which.
//
// THREE SOURCES, IN ONE ORDER, AND THE ORDER IS THE WHOLE RULE.
//
//  1. the deal's own cached card, when one is cached for this reader. Written
//     from a timeline, seats and a deal room, model-authored and citation-checked.
//  2. the night's brief finding for this deal, when the brief surfaced it and no
//     card is cached. Grounded the same way — briefs.AnnotateCurrentRun refuses a
//     finding citing outside the run it annotates — and about this same deal.
//  3. nothing at all, which is the floor and is not a gap.
//
// WHY THE FLOOR IS NOTHING AND NOT A SENTENCE. The plan this implements called
// for a deterministic third arm, and a deterministic STANDING is exactly what
// this queue must not invent: the four standings are a reading of a deal, and
// picking one from a level and a quiet-days count would be this pass deciding a
// judgement that dealstatus owns — the second answer to one question that the
// tree's own rule forbids. What the row keeps instead is what it always had:
// its typed `because` reasons and its consequence, which the CLIENT phrases in
// the reader's own language. That is the deterministic explanation. It is
// already on every row, it needs no standing word to be useful, and a row
// reaching the reader with no verdict is therefore still fully explained.
//
// So the contract carries TWO source members and both are readings. There is no
// third for the deterministic case, because that case is not a verdict at all: a
// row without one carries its typed `because` reasons, the client draws them
// under their own heading, and they are already labelled as rules there.
//
// Held by TestADealWithNeitherAIReadingKeepsItsDeterministicExplanationAndMove.

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealStandings answers the already-written standing for each of these deals.
//
// Its implementation reads a cache and never assembles, which is what makes it
// safe to call with a whole page of ids — the rule DealMoves states, for the
// same reason.
type DealStandings interface {
	CachedStandings(ctx context.Context, dealIDs []ids.UUID) (map[ids.UUID]DealStanding, error)
}

// DealStanding is one deal's written verdict, as much of it as a row draws.
type DealStanding struct {
	Standing     string
	DecisiveLine string
	AsOf         *time.Time
}

// nameTheStanding puts each deal row's already-written standing onto it.
//
// Runs AFTER the page is cut, like nameTheStep and for its reason: a standing is
// drawn and never ranked, so reading one for a row the caller will not receive
// would spend a query on nothing.
//
// The brief findings are handed in rather than read here; feed.go's assembleDay
// states why they travel as a value.
func (s *Service) nameTheStanding(
	ctx context.Context, queue []crmcontracts.WorklistItem, findings map[ids.UUID]string,
) error {
	wanted := dealsWantingAStanding(queue)
	if len(wanted) == 0 {
		return nil
	}
	cached := map[ids.UUID]DealStanding{}
	if s.dealStandings != nil {
		var err error
		cached, err = s.dealStandings.CachedStandings(ctx, wanted)
		if err != nil {
			return err
		}
	}
	for i := range queue {
		id, ok := needsDealStanding(queue[i])
		if !ok {
			continue
		}
		if standing, found := cached[id]; found {
			queue[i].Verdict = verdictOf(standing)
			continue
		}
		if finding := findings[id]; finding != "" {
			queue[i].Verdict = &crmcontracts.WorklistDealVerdict{
				Line:   finding,
				Source: crmcontracts.WorklistInsightSourceBriefFinding,
			}
		}
	}
	return nil
}

// dealsWantingAStanding collects the deals on this page, deduplicated.
func dealsWantingAStanding(queue []crmcontracts.WorklistItem) []ids.UUID {
	wanted := make([]ids.UUID, 0, len(queue))
	seen := map[ids.UUID]bool{}
	for i := range queue {
		id, ok := needsDealStanding(queue[i])
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		wanted = append(wanted, id)
	}
	return wanted
}

// needsDealStanding answers which deal a row is about, where it is about one.
//
// Unlike needsDealMove this does not skip a row that already carries a verdict:
// nothing else writes this field, so there is never one to preserve.
func needsDealStanding(item crmcontracts.WorklistItem) (ids.UUID, bool) {
	if item.Subject == nil || item.Subject.Type != subjectDeal {
		return ids.UUID{}, false
	}
	return ids.UUID(item.Subject.Id), true
}

// verdictOf carries one cached standing onto the wire.
//
// A standing word this build does not recognise is dropped WHOLE rather than
// served: the enum is what a client's copy and its colour turn on, and an
// unrecognised value would reach the browser as a verdict it cannot draw. The
// row then falls through to its typed reasons, which is the floor this pass is
// allowed to leave a row at.
func verdictOf(standing DealStanding) *crmcontracts.WorklistDealVerdict {
	word, ok := knownStanding(standing.Standing)
	if !ok || standing.DecisiveLine == "" {
		return nil
	}
	return &crmcontracts.WorklistDealVerdict{
		Standing: &word,
		Line:     standing.DecisiveLine,
		Source:   crmcontracts.WorklistInsightSourceDealStatus,
		AsOf:     standing.AsOf,
	}
}

// knownStanding answers whether the card's verdict word is one this contract
// carries.
//
// A word this build does not know falls to `default` and is dropped — NOT a
// compile error, because a switch over a string-typed enum is never one and the
// `exhaustive` linter is not enabled in this tree. What fails instead is
// backend/gates/worklistverdictstandings_test.go, which compares this switch,
// the card's own description and the queue's enum and refuses any two that
// disagree. That gate is the mechanism; this function is one of the three
// spellings it holds together.
func knownStanding(word string) (crmcontracts.WorklistDealVerdictStanding, bool) {
	switch crmcontracts.WorklistDealVerdictStanding(word) {
	case crmcontracts.WorklistStandingLive:
		return crmcontracts.WorklistStandingLive, true
	case crmcontracts.WorklistStandingDrifting:
		return crmcontracts.WorklistStandingDrifting, true
	case crmcontracts.WorklistStandingBlocked:
		return crmcontracts.WorklistStandingBlocked, true
	case crmcontracts.WorklistStandingCold:
		return crmcontracts.WorklistStandingCold, true
	default:
		return "", false
	}
}

// findingsOf collects the night's finding per deal out of the brief lane's
// entries.
//
// A pure function of the queue handed in, returned to its caller rather than
// stored: feed.go's assembleDay states why anything per-read on the Service
// crosses readers.
func findingsOf(queue []BriefEntry) map[ids.UUID]string {
	findings := make(map[ids.UUID]string, len(queue))
	for _, entry := range queue {
		if entry.Finding == "" {
			continue
		}
		findings[entry.DealID] = entry.Finding
	}
	return findings
}
