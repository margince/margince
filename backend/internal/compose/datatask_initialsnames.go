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
// Only the stored original of the minting mail proves the name came from the
// address, so a contact whose original is gone keeps its name.
func initialsNamesDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792896243_initials_display_names", Version: 1,
		Title: "Showing initials-only addresses as they are instead of a made-up name",
		Steps: []datatask.Step{{
			Name: "contacts", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateInitialsNames, Batch: repairInitialsNamesBatch,
		}},
	}
}

// repairInitialsNamesBatch walks contacts by id, a page at a time. A batch
// inspects at most its limit and reads one stored original per candidate.
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
		header, proven, err := mintingHeaderName(ctx, tx, candidate)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if !proven {
			continue
		}
		moved, err := contacts.RepairInitialsDisplayNameTx(ctx, tx, candidate, header)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if moved {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, page.Scanned, changed)
}

// mintingHeaderName reads the display name the minting mail's header gave the
// candidate's address. proven is false when the original is gone, is not an
// RFC822 message, or does not name the address: the name is then kept.
func mintingHeaderName(ctx context.Context, tx pgx.Tx, candidate contacts.InitialsNameCandidate) (name string, proven bool, err error) {
	switch candidate.Connector {
	case sourceGmail, sourceIMAP, sourceGraph:
	default:
		return "", false, nil
	}
	payload, found, err := readStoredOriginal(ctx, tx, candidate.RawCaptureID)
	if err != nil || !found {
		return "", false, err
	}
	raw, decodeErr := capture.DecodeStoredOriginal(payload)
	if decodeErr != nil {
		return "", false, nil //nolint:nilerr // an unreadable original proves nothing, so the name stays
	}
	name, proven, parseErr := mailmap.CounterpartyNameOf(raw, candidate.Address)
	if parseErr != nil {
		return "", false, nil //nolint:nilerr // an unreadable original proves nothing, so the name stays
	}
	return name, proven, nil
}

func estimateInitialsNames(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountContactsWithAddress(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
