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
	report, err := s.inverses.perform(marked, s.pool, row, kind, ifVersion)
	if err != nil {
		return privacy.RecordHistoryEntry{}, inverseWriteRefusal(err)
	}
	entry, err := s.readRestoreEntry(ctx, entityType, id, row.ID)
	if err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	entry.LeftBehind = report.LeftBehind
	return entry, nil
}

// perform runs the module verb that undoes the entry, and answers what an
// un-archive could not bring back with the record.
func (r recordInverses) perform(
	ctx context.Context, pool *pgxpool.Pool, row AuditRow, kind inverse, ifVersion int64,
) (storekit.RestoreReport, error) {
	var report storekit.RestoreReport
	var err error
	switch kind {
	case inverseArchive, inverseRearchive:
		err = r.archiveCreated(ctx, pool, row, ifVersion)
	case inverseUnarchive:
		err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			var err error
			report, err = r.unarchive(ctx, tx, row, ifVersion)
			return err
		})
	case inverseDemote:
		_, err = r.contacts.DemoteLead(ctx, ids.From[ids.LeadKind](row.EntityID), demoteReason,
			contacts.OnlyAtVersion(&ifVersion), contacts.NotWorkedOnByAColleagueSince(row.OccurredAt, row.ID))
	case inverseRetractFill:
		fill, _ := fillOf(row)
		err = r.contacts.RetractFill(ctx, ids.From[ids.ContactKind](row.EntityID), fill, &ifVersion)
	case inverseNone:
		err = fmt.Errorf("compose: entry %s has no module verb that undoes it", row.ID)
	}
	return report, err
}

// archiveCreated archives the record a create made, or archives again one an
// un-archive brought back. For a contact, company or
// deal the archive retires its links in the same statement set, and a link a
// colleague added after the decision does not move the record's version, so
// the colleague check is asked again AFTER the archive, in its transaction: a
// link the archive retired is visible to it, and the whole write rolls back.
// An activity archives through its module's own entry point: it retires no
// links, and a colleague relinking it moves its version.
func (r recordInverses) archiveCreated(ctx context.Context, pool *pgxpool.Pool, row AuditRow, ifVersion int64) error {
	var archive func(tx pgx.Tx) error
	switch row.EntityType {
	case entityTypeContact:
		archive = func(tx pgx.Tx) error {
			return r.contacts.ArchiveContactTx(ctx, tx, ids.From[ids.ContactKind](row.EntityID), &ifVersion)
		}
	case entityTypeCompany:
		archive = func(tx pgx.Tx) error {
			return r.contacts.ArchiveCompanyTx(ctx, tx, ids.From[ids.CompanyKind](row.EntityID), &ifVersion)
		}
	case entityTypeDeal:
		archive = func(tx pgx.Tx) error {
			return r.deals.ArchiveDealTx(ctx, tx, ids.From[ids.DealKind](row.EntityID), &ifVersion)
		}
	default:
		_, err := r.provider.ArchiveAt(ctx, datasource.ArchiveInput{
			Ref:       datasource.EntityRef{Type: datasource.EntityType(row.EntityType), ID: row.EntityID},
			IfVersion: &ifVersion,
		})
		return err
	}
	// Asked before the archive, and again after it: a link a colleague added
	// meanwhile is one the archive retired, and a tag or list they added is
	// one the archive's own cascade names.
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := refuseColleagueWork(ctx, tx, row); err != nil {
			return err
		}
		if err := archive(tx); err != nil {
			return err
		}
		if err := refuseColleagueWork(ctx, tx, row); err != nil {
			return err
		}
		dropped, err := contacts.ArchiveDroppedColleagueWork(ctx, tx, row.EntityType, row.EntityID)
		if err != nil {
			return err
		}
		if dropped {
			return RefusedRestore{Reason: ReasonSuperseded, Detail: "a colleague tagged or listed it since"}
		}
		return nil
	})
}

// refuseColleagueWork refuses an archive of a record a colleague has worked on
// since the entry being undone.
func refuseColleagueWork(ctx context.Context, tx pgx.Tx, row AuditRow) error {
	worked, err := contacts.ColleagueWorkedOnSince(ctx, tx, row.EntityType, row.EntityID, row.OccurredAt, row.ID)
	if err != nil {
		return err
	}
	if worked {
		return RefusedRestore{Reason: ReasonSuperseded, Detail: "changed by a colleague since"}
	}
	return nil
}

// unarchive brings the record back through its own module's un-archive.
func (r recordInverses) unarchive(ctx context.Context, tx pgx.Tx, row AuditRow, ifVersion int64) (storekit.RestoreReport, error) {
	with := unarchiveWith(nil)
	switch row.EntityType {
	case entityTypeContact:
		return r.contacts.RestoreContactTx(ctx, tx, ids.From[ids.ContactKind](row.EntityID), &ifVersion, with)
	case entityTypeCompany:
		return r.contacts.RestoreCompanyTx(ctx, tx, ids.From[ids.CompanyKind](row.EntityID), &ifVersion, with)
	case entityTypeDeal:
		return r.deals.RestoreDealTx(ctx, tx, ids.From[ids.DealKind](row.EntityID), &ifVersion, with)
	}
	return storekit.RestoreReport{}, fmt.Errorf("compose: a %s has no un-archive", row.EntityType)
}

// inverseWriteRefusal renders a module's refusal as the reason the button would
// have shown, so a refusal discovered at the write reads the same as one shown
// up front. Anything else is returned as it came.
func inverseWriteRefusal(err error) error {
	var unarchive *storekit.RestoreRefusal
	var fill *contacts.FillRetractionRefusal
	var notPromoted *contacts.NotPromotedError
	var hasDeal *contacts.ContactHasDealError
	var touched *contacts.HumanTouchedError
	switch {
	case errors.As(err, &touched):
		return RefusedRestore{Reason: ReasonSuperseded, Detail: "changed by a colleague since"}
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
