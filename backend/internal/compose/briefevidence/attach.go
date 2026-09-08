// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefevidence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Reader answers the canonical email row of each activity, for the caller in
// ctx. Ids that are not emails, that this caller may not read, or that do not
// exist come back absent rather than as an error — the reader's own gate is
// what decides, and a page must not fail over one row a reader was never
// entitled to.
//
// activities.Store satisfies this directly; InTx binds the batch form for a
// caller holding an open transaction.
type Reader interface {
	EmailSummariesByID(
		ctx context.Context, activityIDs []ids.UUID,
	) (map[ids.UUID]crmcontracts.EmailSummary, error)
}

// ReaderFunc adapts a plain function to Reader.
type ReaderFunc func(
	ctx context.Context, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error)

// EmailSummariesByID satisfies Reader.
func (f ReaderFunc) EmailSummariesByID(
	ctx context.Context, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	return f(ctx, activityIDs)
}

// InTx reads inside a transaction the caller already holds, so the enrichment
// sees the same snapshot as the assembly around it and spends no second
// connection. Bind it to activities.EmailSummariesByIDBatch.
//
//nolint:ireturn // Reader IS the product: the caller holds a transaction and wants the one interface Attach takes, and a concrete type here would be a second name for the same thing.
func InTx(
	tx pgx.Tx,
	batch func(context.Context, pgx.Tx, []ids.UUID) (map[ids.UUID]crmcontracts.EmailSummary, error),
) Reader {
	return ReaderFunc(func(
		ctx context.Context, activityIDs []ids.UUID,
	) (map[ids.UUID]crmcontracts.EmailSummary, error) {
		return batch(ctx, tx, activityIDs)
	})
}

// FromActivities answers out of rows the caller is already holding, with no
// read at all.
//
// The deal status card is the case: it has just assembled the deal's timeline
// under the caller's own scope, and every row in it carries the canonical
// summary the reader is entitled to. Reading them a second time would ask the
// same question of the same gate and get the same answer, one statement later.
//
//nolint:ireturn // same as InTx — this exists to produce a Reader out of rows already held.
func FromActivities(rows []crmcontracts.Activity) Reader {
	held := make(map[ids.UUID]crmcontracts.EmailSummary, len(rows))
	for _, row := range rows {
		if row.EmailSummary != nil {
			held[ids.UUID(row.Id)] = *row.EmailSummary
		}
	}
	return ReaderFunc(func(
		_ context.Context, activityIDs []ids.UUID,
	) (map[ids.UUID]crmcontracts.EmailSummary, error) {
		out := make(map[ids.UUID]crmcontracts.EmailSummary, len(activityIDs))
		for _, id := range activityIDs {
			if summary, ok := held[id]; ok {
				out[id] = summary
			}
		}
		return out, nil
	})
}

// Target is one cited activity and the field on the enclosing wire row that
// receives its summary.
//
// A setter rather than a pointer to the field, because the producers do not
// share one evidence type: the brief's citation and the deal move's basis are
// different schemas that carry the same email. The setter is what lets one
// batching rule serve both without reflection and without a second copy.
type Target struct {
	activityID ids.UUID
	set        func(*crmcontracts.EmailSummary)
}

// Attach fills every target's summary from one read.
//
// The ids are deduped, so a response citing one message from four sentences
// asks for it once; zero targets means zero calls. A nil reader is a no-op for
// a service constructed without one in a unit test — every production
// construction wires one, and compose's assembly test is what holds that.
//
// Evidence is NOT filtered by kind before the read, deliberately: a citation
// carries a record id and no kind word, so the only honest answer to "is this
// an email" is the reader's own `a.kind = 'email'` clause. Filtering here would
// need a second, guessed answer to the question the reader already settles.
//
// A reader error fails the response rather than dropping the summaries: the
// alternative is a page that silently degrades to unopenable citations, which
// reads to the user exactly like the defect this package removes. Same rule
// search states for its own summaries.
func Attach(ctx context.Context, reader Reader, targets []Target) error {
	if reader == nil || len(targets) == 0 {
		return nil
	}
	seen := make(map[ids.UUID]bool, len(targets))
	wanted := make([]ids.UUID, 0, len(targets))
	for _, target := range targets {
		if seen[target.activityID] {
			continue
		}
		seen[target.activityID] = true
		wanted = append(wanted, target.activityID)
	}
	summaries, err := reader.EmailSummariesByID(ctx, wanted)
	if err != nil {
		return fmt.Errorf("brief evidence: reading this response's email rows: %w", err)
	}
	for _, target := range targets {
		summary, ok := summaries[target.activityID]
		if !ok {
			continue
		}
		// Copied into a local first: taking the address of the range variable
		// would point every row at the last summary read.
		held := summary
		target.set(&held)
	}
	return nil
}

// evidenceTargets collects the activity citations out of one evidence slice,
// addressing the real elements so the setter writes back into the response.
func evidenceTargets(out *[]Target, evidence []crmcontracts.CompanyBriefEvidence) {
	for i := range evidence {
		row := &evidence[i]
		if row.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
			continue
		}
		*out = append(*out, Target{
			activityID: ids.UUID(row.EntityId),
			set:        func(summary *crmcontracts.EmailSummary) { row.EmailSummary = summary },
		})
	}
}

// FromEvidence is the collector for a bare evidence slice — a growth-fit
// sub-score's sources, one sentence's citations on their own.
func FromEvidence(evidence []crmcontracts.CompanyBriefEvidence) []Target {
	var targets []Target
	evidenceTargets(&targets, evidence)
	return targets
}

// FromEvidenceRef is the collector for a citation a row holds by pointer — a
// work attention's single receipt, a draft reason's one record.
//
// It takes the pointer rather than a copy for the reason the whole package
// takes addresses: an enriched copy is thrown away, and the response ships the
// bare citation while every test over the collector passes.
func FromEvidenceRef(row *crmcontracts.CompanyBriefEvidence) []Target {
	if row == nil || row.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
		return nil
	}
	return []Target{{
		activityID: ids.UUID(row.EntityId),
		set:        func(summary *crmcontracts.EmailSummary) { row.EmailSummary = summary },
	}}
}

// FromSentences is the collector for grounded prose: the brief's sections, the
// dossier's, the answer to a prepared question, the deal card's story.
func FromSentences(sentences []crmcontracts.CompanyBriefSentence) []Target {
	var targets []Target
	for i := range sentences {
		evidenceTargets(&targets, sentences[i].Evidence)
	}
	return targets
}

// FromSuggestions is the collector for the account page's advice, whose
// evidence is what the rule fired on.
func FromSuggestions(suggestions []crmcontracts.Company360Suggestion) []Target {
	var targets []Target
	for i := range suggestions {
		evidenceTargets(&targets, suggestions[i].Evidence)
	}
	return targets
}

// FromReasons is the collector for a generated draft's "why this draft?" list,
// where each reason cites at most one record.
func FromReasons(reasons []crmcontracts.AccountDraftReason) []Target {
	var targets []Target
	for i := range reasons {
		targets = append(targets, FromEvidenceRef(reasons[i].EvidenceRef)...)
	}
	return targets
}

// FromDealMove is the collector for the deal card's recommended move, whose
// basis is its own wire shape rather than the shared citation.
func FromDealMove(basis []crmcontracts.DealNextBestActionEvidence) []Target {
	var targets []Target
	for i := range basis {
		row := &basis[i]
		if row.ActivityId == nil {
			continue
		}
		targets = append(targets, Target{
			activityID: ids.UUID(*row.ActivityId),
			set:        func(summary *crmcontracts.EmailSummary) { row.EmailSummary = summary },
		})
	}
	return targets
}
