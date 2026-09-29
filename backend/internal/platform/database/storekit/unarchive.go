// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Bringing an archived record back, with what its archive took down.
//
// The archive recorded every child row it retired and every membership and tag
// it deleted on its own audit row (archivecascade.go). An un-archive reads that
// row back and puts each one back where it still can. A row that cannot come
// back — its list or tag archived since, a link whose other end is archived, a
// value another record holds now — is left behind and named on the restore's
// own audit row, while the record itself comes back.
//
// What refuses the whole un-archive is what would make the record a different
// one than was archived: it was merged away, its personal data was erased
// since, or another live record has taken its unique value, which the unique
// indexes would refuse anyway.
//
// The flow is here, once; every statement it runs is the owning module's, in
// its UnarchiveShape, so each write stays a literal in the module that owns
// the table.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UnarchiveShape is one record type's share of an un-archive.
type UnarchiveShape struct {
	Table string
	// Lock answers label, archived_at, whether the record was merged away, and
	// version, under FOR UPDATE.
	Lock string
	// Taken answers the first unique value of a row retired from TakenFrom
	// (ids in $1, retired at $2) that a live record holds now. Empty for a type
	// with no such value.
	Taken, TakenFrom, TakenField string
	// Children restore one retired row ($1) retired at $2, in this order.
	Children []ChildRestore
	// Membership re-inserts one list_member row (@list_id, @entity_type,
	// @entity_id, @added_by, @created_at, @note) and records it in
	// list_member_event as restored by @actor; Tag re-inserts one taggable row.
	Membership, Tag string
	Restored        events.Payload
}

// ChildRestore is the statement that brings back one row of Table.
type ChildRestore struct{ Table, Statement string }

// RestoreWith is what an un-archive needs from its caller besides the record.
type RestoreWith struct {
	// Erased is required: restoring from behind an erasure would bring back
	// what the erasure certified gone.
	Erased ErasedSince
	// PendingLinks are links an earlier restore of the same change left
	// behind because this record was still archived. Each is tried again once
	// this record is live, and one that comes back is credited to it.
	PendingLinks []LeftBehind
}

// Unarchive asks the refusals, and then writes the record, what it can of its
// cascade, the restore audit row and the event. The authority gates are the
// caller's: each module asks its own object and row gates before it gets here.
func Unarchive(
	ctx context.Context, tx pgx.Tx, shape UnarchiveShape, id ids.UUID, ifVersion *int64, with RestoreWith,
) (RestoreReport, error) {
	erased := with.Erased
	if erased == nil {
		return RestoreReport{}, errors.New("store: an un-archive needs the erasure boundary to ask")
	}
	var report RestoreReport
	var archivedAt *time.Time
	var merged bool
	var version int64
	err := tx.QueryRow(ctx, shape.Lock, id).Scan(&report.Label, &archivedAt, &merged, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return RestoreReport{}, apperrors.ErrNotFound
	}
	if err != nil {
		return RestoreReport{}, fmt.Errorf("lock %s %s to restore it: %w", shape.Table, id, err)
	}
	archive, err := refuseUnarchive(ctx, tx, shape, id, archivedAt, merged, erased)
	if err != nil {
		return RestoreReport{}, err
	}
	if ifVersion != nil && *ifVersion != version {
		return RestoreReport{}, apperrors.ErrVersionSkew
	}
	if err := refuseTakenValue(ctx, tx, shape, archive.Cascade, *archivedAt); err != nil {
		return RestoreReport{}, err
	}
	p := NewPatch()
	p.Set("archived_at", *archivedAt, nil)
	if err := p.ApplyGuardedIn(ctx, tx, shape.Table, id, ifVersion, IncludeArchived); err != nil {
		return RestoreReport{}, err
	}
	if report.LeftBehind, err = restoreCascade(ctx, tx, shape, id, archive.Cascade, *archivedAt); err != nil {
		return RestoreReport{}, err
	}
	if report.Relinked, err = relink(ctx, tx, shape, with.PendingLinks); err != nil {
		return RestoreReport{}, err
	}
	auditID, err := AuditWithEvidence(ctx, tx, "restore", shape.Table, id,
		p.Before(), p.After(), report.Evidence(archive.AuditID))
	if err != nil {
		return RestoreReport{}, err
	}
	return report, EmitEvent(ctx, tx, auditID, id, shape.Restored)
}

// refuseUnarchive answers the archive the record comes back from, or why it
// cannot come back at all.
func refuseUnarchive(
	ctx context.Context, tx pgx.Tx, shape UnarchiveShape, id ids.UUID,
	archivedAt *time.Time, merged bool, erased ErasedSince,
) (ArchiveRecord, error) {
	if merged {
		return ArchiveRecord{}, &RestoreRefusal{Reason: RestoreMerged}
	}
	if archivedAt == nil {
		return ArchiveRecord{}, &RestoreRefusal{Reason: RestoreNotArchived}
	}
	archive, err := LatestArchive(ctx, tx, shape.Table, id)
	if err != nil {
		return ArchiveRecord{}, err
	}
	behind, err := erased(ctx, tx, archive.AuditID)
	if err != nil {
		return ArchiveRecord{}, err
	}
	if behind {
		return ArchiveRecord{}, &RestoreRefusal{Reason: RestoreErased}
	}
	return archive, nil
}

// refuseTakenValue refuses an un-archive whose email or domain a live record
// has taken since. The record is not the record without it, so it is not
// restored without it either.
func refuseTakenValue(ctx context.Context, tx pgx.Tx, shape UnarchiveShape, cascade ArchiveCascade, archivedAt time.Time) error {
	retired := cascade.Retired[shape.TakenFrom]
	if len(retired) == 0 {
		return nil
	}
	var taken string
	err := tx.QueryRow(ctx, shape.Taken, retired, archivedAt).Scan(&taken)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check whether another %s holds this %s: %w", shape.Table, shape.TakenField, err)
	}
	// The field, never the value: the refusal travels into a stored bulk
	// result, and the value is personal data.
	return &RestoreRefusal{Reason: RestoreValueTaken, Detail: shape.TakenField}
}

// restoreCascade puts back each retired row, membership and tag that can still
// come back, and answers the ones that cannot.
func restoreCascade(
	ctx context.Context, tx pgx.Tx, shape UnarchiveShape, id ids.UUID, cascade ArchiveCascade, archivedAt time.Time,
) ([]LeftBehind, error) {
	var left []LeftBehind
	for _, child := range shape.Children {
		for _, row := range cascade.Retired[child.Table] {
			back, err := TryInSavepoint(ctx, tx, child.Statement, row, archivedAt)
			if err != nil {
				return nil, fmt.Errorf("restore %s %s: %w", child.Table, row, err)
			}
			// The record's own email or domain is part of the record: when a
			// live record took it after refuseTakenValue looked, the whole
			// restore is refused rather than committed without it.
			if !back && child.Table == shape.TakenFrom {
				return nil, &RestoreRefusal{Reason: RestoreValueTaken, Detail: shape.TakenField}
			}
			if !back {
				left = append(left, LeftBehind{Kind: child.Table, ID: row, RetiredAt: archivedAt})
			}
		}
	}
	actor, err := CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range cascade.Memberships {
		back, err := TryInSavepoint(ctx, tx, shape.Membership, pgx.StrictNamedArgs{
			"list_id": m.ListID, "entity_type": shape.Table, "entity_id": id,
			"added_by": m.AddedBy, "created_at": m.CreatedAt, "note": m.Note, "actor": actor,
		})
		if err != nil {
			return nil, fmt.Errorf("restore the membership of list %s: %w", m.ListID, err)
		}
		if !back {
			left = append(left, LeftBehind{Kind: "list", ID: m.ListID})
		}
	}
	for _, tag := range cascade.Tags {
		back, err := TryInSavepoint(ctx, tx, shape.Tag,
			tag.TagID, shape.Table, id, tag.AssignedBy, tag.AssignedByKind, tag.AssignedAt)
		if err != nil {
			return nil, fmt.Errorf("restore tag %s: %w", tag.TagID, err)
		}
		if !back {
			left = append(left, LeftBehind{Kind: "tag", ID: tag.TagID})
		}
	}
	return left, nil
}

// relink tries each pending link again now that this record is live, and
// answers the ones that came back.
func relink(ctx context.Context, tx pgx.Tx, shape UnarchiveShape, pending []LeftBehind) ([]ids.UUID, error) {
	var back []ids.UUID
	for _, link := range pending {
		for _, child := range shape.Children {
			if child.Table != link.Kind {
				continue
			}
			restored, err := TryInSavepoint(ctx, tx, child.Statement, link.ID, link.RetiredAt)
			if err != nil {
				return nil, fmt.Errorf("restore %s %s: %w", link.Kind, link.ID, err)
			}
			if restored {
				back = append(back, link.ID)
			}
		}
	}
	return back, nil
}
