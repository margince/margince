// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// initialsNamesDataTask shows an initials-only address as its local part ("dv"
// for dv@) on contacts capture named "Dv" before the parser learned that rule.
// Only the stored originals of the mails capture read as it created the contact
// prove the name came from the address. Without one, the name stays.
func initialsNamesDataTask() datatask.Task {
	return datatask.Task{
		ID: contacts.InitialsRepairTaskID, Version: 1,
		Title: "Showing initials-only addresses as they are instead of a made-up name",
		Steps: []datatask.Step{{
			Name: "contacts", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateInitialsNames, Batch: repairInitialsNamesBatch,
		}},
	}
}

// repairInitialsNamesBatch walks contacts by id, a page at a time. A batch
// inspects at most its limit, and each candidate costs it a bounded number of
// stored originals.
func repairInitialsNamesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.InitialsNamePageTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, candidate := range page.Candidates {
		headers, err := headerNamesOf(ctx, tx, candidate)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		moved, err := contacts.RepairInitialsDisplayNameTx(ctx, tx, candidate, headers)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if moved {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, page.Scanned, changed)
}

// headerNamesOf reads the display name each of the candidate's stored originals
// gave its address. An original that is gone, is not an RFC822 message, or
// does not list the address proves nothing either way and adds no name.
func headerNamesOf(ctx context.Context, tx pgx.Tx, candidate contacts.InitialsNameCandidate) ([]string, error) {
	var names []string
	for _, evidence := range candidate.Evidence {
		switch evidence.Connector {
		case sourceGmail, sourceIMAP, sourceGraph:
		default:
			continue
		}
		payload, found, err := readStoredOriginal(ctx, tx, evidence.RawCaptureID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		raw, decodeErr := capture.DecodeStoredOriginal(payload)
		if decodeErr != nil {
			continue
		}
		name, listed, parseErr := mailmap.CounterpartyNameOf(raw, candidate.Address, evidence.Outbound)
		if parseErr != nil || !listed {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func estimateInitialsNames(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountContactsWithAddress(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
