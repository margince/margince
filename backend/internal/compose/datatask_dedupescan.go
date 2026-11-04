// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dedupeScanRequiredMigration supplies the company name-key indexes that keep
// one company's check from reading the whole table.
const dedupeScanRequiredMigration = "core/1792855916"

// dedupeScanDataTask runs every company and contact through the duplicate
// check once, so pairs written before a lane existed reach the review queue.
// It only files review pairs and merges nothing. A later change to the filing
// rule ships a new task ID, which runs the full pass again.
func dedupeScanDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792855917_dedupe_scan", Version: 1,
		Title: "Finding duplicates among existing records", Requires: []string{dedupeScanRequiredMigration},
		Steps: []datatask.Step{
			dedupeScanStep("companies", contacts.DedupeScanCompanies),
			dedupeScanStep("contacts", contacts.DedupeScanContacts),
		},
	}
}

// gmailBaseRequiredMigration supplies the index the address-variant lane reads
// once Gmail spellings share one base.
const gmailBaseRequiredMigration = "core/1793773371"

// gmailDedupeScanDataTask runs every contact through the duplicate check once
// more, so two contacts stored under two Gmail spellings of one mailbox reach
// the review queue. The company check reads no mail address, so its pass is
// not repeated.
func gmailDedupeScanDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793773372_gmail_dedupe_scan", Version: 1,
		Title: "Finding contacts on two spellings of one Gmail address", Requires: []string{gmailBaseRequiredMigration},
		Steps: []datatask.Step{dedupeScanStep("contacts", contacts.DedupeScanContacts)},
	}
}

// dedupeScanStep is one record type's pass. Each pair filed writes its own
// audit row (recordDedupeCandidate).
func dedupeScanStep(name string, entity contacts.DedupeScanEntity) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 100, MinLimit: 10,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := contacts.CountDedupeScanSubjects(ctx, q, entity, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			if err := auth.RequireSystem(ctx); err != nil {
				return datatask.BatchOut{}, err
			}
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			page, err := contacts.ScanForDuplicatesTx(ctx, tx, capture.MatcherTx, entity, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, page.Scanned, page.Filed)
		},
	}
}
