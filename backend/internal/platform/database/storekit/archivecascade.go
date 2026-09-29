// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// What an archive took down with its record, and the shared half of putting it
// back.
//
// Archiving a contact, company or deal retires child rows and deletes the
// record's list memberships and tags, which have no archived_at. The archive's
// own audit row is the one place that survives, so it records all of it in its
// evidence, and an un-archive reads it back from there. The statements that
// retire and restore stay in each owning module; this file holds the shape they
// agree on.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The evidence keys an archive and its un-archive write.
const (
	// EvidenceKeyArchiveCascade holds an ArchiveCascade on an archive row.
	EvidenceKeyArchiveCascade = "archive_cascade"
	// EvidenceKeyRestoresArchive names, on a restore row, the archive row it
	// reversed.
	EvidenceKeyRestoresArchive = "restores_archive"
	// EvidenceKeyLeftBehind lists, on a restore row, what could not come back.
	EvidenceKeyLeftBehind = "left_behind"
	// EvidenceKeyRelinked lists, on a restore row, links an earlier restore of
	// the same change left behind that this one brought back.
	EvidenceKeyRelinked = "relinked"
)

// ArchiveCascade is everything one archive changed besides the record's row.
type ArchiveCascade struct {
	// Retired names, per child table, the rows the archive stamped archived.
	Retired     map[string][]ids.UUID `json:"retired,omitempty"`
	Memberships []ListMembership      `json:"list_memberships,omitempty"`
	Tags        []TagAssignment       `json:"tags,omitempty"`
}

// ListMembership is one list_member row an archive deleted.
type ListMembership struct {
	ListID    ids.UUID  `json:"list_id"`
	AddedBy   string    `json:"added_by"`
	CreatedAt time.Time `json:"created_at"`
}

// TagAssignment is one taggable row an archive deleted.
type TagAssignment struct {
	TagID          ids.UUID  `json:"tag_id"`
	AssignedBy     *ids.UUID `json:"assigned_by,omitempty"`
	AssignedByKind *string   `json:"assigned_by_kind,omitempty"`
	AssignedAt     time.Time `json:"assigned_at"`
}

// Evidence is the cascade as the archive audit row's evidence.
func (c ArchiveCascade) Evidence() map[string]any {
	return map[string]any{EvidenceKeyArchiveCascade: c}
}

// Retire runs one retiring statement that answers the ids it stamped, and
// files them under table.
func (c *ArchiveCascade) Retire(ctx context.Context, tx pgx.Tx, table, statement string, args ...any) error {
	rows, err := tx.Query(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("retire %s: %w", table, err)
	}
	retired, err := ScanUUIDColumn(rows, "retire "+table)
	if err != nil {
		return err
	}
	if len(retired) > 0 {
		if c.Retired == nil {
			c.Retired = map[string][]ids.UUID{}
		}
		c.Retired[table] = retired
	}
	return nil
}

// DropMemberships runs a DELETE on list_member that answers list_id, added_by
// and created_at, and keeps what it deleted.
func (c *ArchiveCascade) DropMemberships(ctx context.Context, tx pgx.Tx, statement string, id ids.UUID) error {
	rows, err := tx.Query(ctx, statement, id)
	if err != nil {
		return fmt.Errorf("drop list memberships: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m ListMembership
		if err := rows.Scan(&m.ListID, &m.AddedBy, &m.CreatedAt); err != nil {
			return fmt.Errorf("drop list memberships: %w", err)
		}
		c.Memberships = append(c.Memberships, m)
	}
	return rows.Err()
}

// DropTags runs a DELETE on taggable that answers tag_id, assigned_by,
// assigned_by_kind and assigned_at, and keeps what it deleted.
func (c *ArchiveCascade) DropTags(ctx context.Context, tx pgx.Tx, statement string, id ids.UUID) error {
	rows, err := tx.Query(ctx, statement, id)
	if err != nil {
		return fmt.Errorf("drop tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tag TagAssignment
		if err := rows.Scan(&tag.TagID, &tag.AssignedBy, &tag.AssignedByKind, &tag.AssignedAt); err != nil {
			return fmt.Errorf("drop tags: %w", err)
		}
		c.Tags = append(c.Tags, tag)
	}
	return rows.Err()
}

// ArchiveRecord is the newest archive audit row of one record.
type ArchiveRecord struct {
	AuditID ids.UUID
	Cascade ArchiveCascade
}

// LatestArchive reads the archive row an un-archive of this record reverses.
// An archive written before archives recorded their cascade reads as an empty
// one: the record comes back, and nothing else was kept to bring.
func LatestArchive(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) (ArchiveRecord, error) {
	var out ArchiveRecord
	var cascade []byte
	err := tx.QueryRow(ctx,
		`SELECT id, evidence -> $3 FROM audit_log
		  WHERE entity_type = $1 AND entity_id = $2 AND action = 'archive'
		  ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		entityType, id, EvidenceKeyArchiveCascade).Scan(&out.AuditID, &cascade)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArchiveRecord{}, &RestoreRefusal{Reason: RestoreNoArchiveRecord}
	}
	if err != nil {
		return ArchiveRecord{}, fmt.Errorf("read the archive of %s %s: %w", entityType, id, err)
	}
	if len(cascade) > 0 {
		if err := json.Unmarshal(cascade, &out.Cascade); err != nil {
			return ArchiveRecord{}, fmt.Errorf("read what the archive of %s %s took down: %w", entityType, id, err)
		}
	}
	return out, nil
}

// ErasedSince answers whether an erasure of the audited record was certified
// after the audit row auditID. The scrub vocabulary is the privacy module's,
// which no record module may import, so compose supplies it.
type ErasedSince func(ctx context.Context, tx pgx.Tx, auditID ids.UUID) (bool, error)

// RestoreReason names why a record cannot be un-archived.
type RestoreReason string

// The refusals an un-archive answers.
const (
	RestoreMerged          RestoreReason = "merged"
	RestoreErased          RestoreReason = "erased"
	RestoreValueTaken      RestoreReason = "value_taken"
	RestoreNotArchived     RestoreReason = "not_archived"
	RestoreNoArchiveRecord RestoreReason = "no_archive_record"
)

// RestoreRefusal is an un-archive a rule refuses. It is a conflict with the
// record's current state, never a fault.
type RestoreRefusal struct {
	Reason RestoreReason
	// Detail names what was refused, such as the address another record holds.
	Detail string
}

func (e *RestoreRefusal) Error() string {
	if e.Detail == "" {
		return "the record cannot be restored: " + string(e.Reason)
	}
	return fmt.Sprintf("the record cannot be restored: %s (%s)", e.Reason, e.Detail)
}

func (e *RestoreRefusal) Unwrap() error { return apperrors.ErrConflict }

// LeftBehind is one thing an un-archive could not bring back: a child row whose
// value another record has taken since, a list or tag since archived.
type LeftBehind struct {
	Kind string   `json:"kind"`
	ID   ids.UUID `json:"id"`
	// RetiredAt is the stamp the archive left on the row, which a later try
	// matches on. It stays out of the audit evidence.
	RetiredAt time.Time `json:"-"`
}

// RestoreReport is what one un-archive did beyond the record's own row.
type RestoreReport struct {
	Label      string
	LeftBehind []LeftBehind
	// Relinked are pending links of the same change this restore brought back.
	Relinked []ids.UUID
}

// Evidence is what a restore row records: the archive it reversed, and what
// did not come back.
func (r RestoreReport) Evidence(archive ids.UUID) map[string]any {
	evidence := map[string]any{EvidenceKeyRestoresArchive: archive}
	if len(r.LeftBehind) > 0 {
		evidence[EvidenceKeyLeftBehind] = r.LeftBehind
	}
	if len(r.Relinked) > 0 {
		evidence[EvidenceKeyRelinked] = r.Relinked
	}
	return evidence
}

// TryInSavepoint runs one write in a savepoint, and answers false when the
// write changed no row or a constraint refused it. Only a constraint refusal
// (SQLSTATE class 23) is rolled back and swallowed as "not applied"; any other
// error is the caller's.
func TryInSavepoint(ctx context.Context, tx pgx.Tx, statement string, args ...any) (bool, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("open a savepoint: %w", err)
	}
	tag, err := savepoint.Exec(ctx, statement, args...)
	if err == nil {
		return tag.RowsAffected() > 0, savepoint.Commit(ctx)
	}
	if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
		return false, fmt.Errorf("roll back a savepoint: %w", rollbackErr)
	}
	if isIntegrityRefusal(err) {
		return false, nil
	}
	return false, err
}
