// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Performing an undo that is a module's own verb: archive, un-archive, demote,
// or take a fill back. recordinverse.go decides whether it may run.
//
// Each goes through the owning module's entry point, so the module's grants,
// row scope, write shape and refusals hold without being restated here. The
// context carries the entry being reversed (storekit.WithReversal), which is how
// the module's own audit row comes to name it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// recordInverses holds the module entry points an inverse runs through.
type recordInverses struct {
	provider *Provider
	contacts *contacts.Store
	deals    *deals.Store
}

// demoteReason is what a demotion an undo performs records as its why: the
// demote verb requires one, and "undone from the history" is the honest answer.
const demoteReason = "Undone from the record's history."

// reverseByVerb decides, then performs the module verb, then answers with the
// line that recorded it. The decision checks the path record's version in its
// own transaction, and the write pins the same version, so a change landing in
// between refuses as version skew rather than being undone blind.
func (s RestoreSeam) reverseByVerb(
	ctx context.Context, entityType string, id ids.UUID, row AuditRow, kind inverse, ifVersion int64,
) (privacy.RecordHistoryEntry, error) {
	var answer Undoability
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := recordVersionUnmoved(ctx, tx, entityType, id, ifVersion); err != nil {
			return err
		}
		var err error
		answer, err = s.evaluator.Evaluate(ctx, tx, row, Binding)
		return err
	})
	if errors.Is(err, apperrors.ErrVersionSkew) {
		return privacy.RecordHistoryEntry{}, err
	}
	if err != nil {
		return privacy.RecordHistoryEntry{}, fmt.Errorf("compose: decide whether the change can be undone: %w", err)
	}
	if !answer.Undoable {
		return privacy.RecordHistoryEntry{}, RefusedRestore{Reason: answer.Reason, Detail: answer.Detail}
	}
	marked := storekit.WithReversal(ctx, row.EntityType, row.EntityID, row.ID)
	if err := s.inverses.perform(marked, s.pool, row, kind, ifVersion); err != nil {
		return privacy.RecordHistoryEntry{}, inverseWriteRefusal(err)
	}
	return s.readRestoreEntry(ctx, entityType, id, row.ID)
}

// perform runs the module verb that undoes the entry.
func (r recordInverses) perform(ctx context.Context, pool *pgxpool.Pool, row AuditRow, kind inverse, ifVersion int64) error {
	switch kind {
	case inverseArchive:
		_, err := r.provider.ArchiveAt(ctx, datasource.ArchiveInput{
			Ref:       datasource.EntityRef{Type: datasource.EntityType(row.EntityType), ID: row.EntityID},
			IfVersion: &ifVersion,
		})
		return err
	case inverseUnarchive:
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			return r.unarchive(ctx, tx, row, ifVersion)
		})
	case inverseDemote:
		_, err := r.contacts.DemoteLead(ctx, ids.From[ids.LeadKind](row.EntityID), demoteReason,
			contacts.OnlyAtVersion(&ifVersion))
		return err
	case inverseRetractFill:
		fill, _ := fillOf(row)
		return r.contacts.RetractFill(ctx, ids.From[ids.ContactKind](row.EntityID), fill, &ifVersion)
	case inverseNone:
	}
	return fmt.Errorf("compose: entry %s has no module verb that undoes it", row.ID)
}

// unarchive brings the record back through its own module's un-archive.
func (r recordInverses) unarchive(ctx context.Context, tx pgx.Tx, row AuditRow, ifVersion int64) error {
	with := storekit.RestoreWith{Erased: archiveIsBehindErasure}
	var err error
	switch row.EntityType {
	case entityTypeContact:
		_, err = r.contacts.RestoreContactTx(ctx, tx, ids.From[ids.ContactKind](row.EntityID), &ifVersion, with)
	case string(recordTypeCompany):
		_, err = r.contacts.RestoreCompanyTx(ctx, tx, ids.From[ids.CompanyKind](row.EntityID), &ifVersion, with)
	case entityTypeDeal:
		_, err = r.deals.RestoreDealTx(ctx, tx, ids.From[ids.DealKind](row.EntityID), &ifVersion, with)
	default:
		err = fmt.Errorf("compose: a %s has no un-archive", row.EntityType)
	}
	return err
}

// inverseWriteRefusal renders a module's refusal as the reason the button would
// have shown, so a refusal discovered at the write reads the same as one shown
// up front. Anything else is returned as it came.
func inverseWriteRefusal(err error) error {
	var unarchive *storekit.RestoreRefusal
	var fill *contacts.FillRetractionRefusal
	var notPromoted *contacts.NotPromotedError
	var hasDeal *contacts.ContactHasDealError
	switch {
	case errors.As(err, &unarchive):
		return unarchiveRefusal(unarchive)
	case errors.As(err, &fill):
		answer := fillRefusal(fill)
		return RefusedRestore{Reason: answer.Reason, Detail: answer.Detail}
	case errors.As(err, &notPromoted):
		return RefusedRestore{Reason: ReasonSuperseded, Detail: "the lead is no longer promoted"}
	case errors.As(err, &hasDeal):
		return RefusedRestore{Reason: ReasonNotRestorableByThisPath, Detail: hasDeal.Error()}
	}
	return err
}

// unarchiveRefusal maps storekit's un-archive refusals onto the reasons.
func unarchiveRefusal(refusal *storekit.RestoreRefusal) RefusedRestore {
	switch refusal.Reason {
	case storekit.RestoreErased:
		return RefusedRestore{Reason: ReasonBehindErasureBoundary}
	case storekit.RestoreNotArchived:
		return RefusedRestore{Reason: ReasonSuperseded, Detail: "archived_at"}
	case storekit.RestoreMerged, storekit.RestoreValueTaken, storekit.RestoreNoArchiveRecord:
	}
	return RefusedRestore{Reason: ReasonNotRestorableByThisPath, Detail: string(refusal.Reason) + " " + refusal.Detail}
}
