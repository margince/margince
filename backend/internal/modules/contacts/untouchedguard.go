// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A write that may only land while nobody has touched the record since a given
// instant, re-asked INSIDE the transaction that writes.
//
// The caller that needs it is the import undo: it reads which imported rows a
// human has edited, then archives the ones nobody touched. Those were two
// transactions, so a human could act in the window between them and have their
// edit reversed anyway — the check said untouched, and by the time the archive
// ran it was not. Narrowing the window is not the answer; a precondition the
// WRITE re-asks under its own row lock is, because then the two cannot
// disagree.
//
// It is an option rather than a parameter so no existing caller changes: a verb
// asked without one behaves exactly as it did.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WriteOption is a precondition a caller attaches to a write.
type WriteOption func(*writeOptions)

type writeOptions struct {
	untouchedSince *time.Time
	// untouchedAfter is the audit entry untouchedSince was read from, when the
	// caller has one: ColleagueWorkedOnSince orders by it as well as by time.
	untouchedAfter ids.UUID
	atVersion      *int64
}

// NotTouchedByHumanSince refuses the write when a HUMAN has acted on the record
// since `at`. What counts as acting is any human-actor audit row on the entity,
// not narrowly an update: somebody who independently archived an imported row
// is exactly the case this protects, and a narrower filter would reverse their
// act too.
func NotTouchedByHumanSince(at time.Time) WriteOption {
	return func(o *writeOptions) { o.untouchedSince = &at }
}

// NotWorkedOnByAColleagueSince refuses the write when a colleague has worked
// on the record since the audit entry `entry`, written at `at`, in the sense
// ColleagueWorkedOnSince answers.
func NotWorkedOnByAColleagueSince(at time.Time, entry ids.UUID) WriteOption {
	return func(o *writeOptions) { o.untouchedSince, o.untouchedAfter = &at, entry }
}

// OnlyAtVersion refuses the write unless the record is still at this version —
// the option form of the contract's optional If-Match, asked inside the
// transaction that writes and under the row lock it already holds.
//
// A nil version attaches no precondition, so a caller that has one and a caller
// that does not spell the call the same way. That matters at the seam this
// exists for: an agent write carries the version its approval was released
// against, and an unapproved call at a static tier carries none.
func OnlyAtVersion(v *int64) WriteOption {
	return func(o *writeOptions) { o.atVersion = v }
}

func collectWriteOptions(opts []WriteOption) writeOptions {
	var out writeOptions
	for _, apply := range opts {
		apply(&out)
	}
	return out
}

// refuseIfVersionMoved answers skew when the locked row is no longer at the
// version the caller's authority was granted against.
//
// current comes from a read taken AFTER the row lock, so what it compares is
// the row this transaction is about to write — which is the whole point of the
// pin travelling this far. A check before the lock proves the row was right a
// moment ago and nothing about the write.
func refuseIfVersionMoved(entity string, current *int64, o writeOptions) error {
	if o.atVersion == nil {
		return nil
	}
	// A row that answers no version cannot satisfy a pin, so it refuses rather
	// than passing. The alternative reads as "checked" and is not: an unversioned
	// read would wave through exactly the write the pin was attached to stop.
	if current == nil {
		return &apperrors.VersionSkewError{Message: fmt.Sprintf(
			"This %s has no version to check against version %d, so nothing was changed. "+
				"Read it again, then make the change again if it still applies.",
			entity, *o.atVersion)}
	}
	if *current == *o.atVersion {
		return nil
	}
	return &apperrors.VersionSkewError{Message: fmt.Sprintf(
		"This %s is at version %d, not version %d, so nothing was changed. "+
			"Read it again, then make the change again if it still applies.",
		entity, *current, *o.atVersion)}
}

// HumanTouchedError refuses a write whose caller asked for it only while the
// record was untouched, and it names the record so a report can say which one
// it left alone rather than only how many.
type HumanTouchedError struct {
	EntityType string
	EntityID   ids.UUID
}

func (e *HumanTouchedError) Error() string {
	return fmt.Sprintf("a contact has changed this %s since it was imported, so it was left alone", e.EntityType)
}

// refuseIfHumanTouched enforces the precondition inside the caller's own
// transaction, which is the whole point: the archive that follows takes the
// row's lock, so a human write either lands entirely before this read or
// entirely after the commit, and never between them.
func refuseIfHumanTouched(
	ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, opts writeOptions,
) error {
	if opts.untouchedSince == nil {
		return nil
	}
	var touched bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM audit_log
			 WHERE entity_type = $1 AND entity_id = $2
			   AND actor_type = 'human' AND occurred_at > $3)`,
		entityType, id, *opts.untouchedSince).Scan(&touched); err != nil {
		return fmt.Errorf("checking whether a contact has changed this %s: %w", entityType, err)
	}
	if touched {
		return &HumanTouchedError{EntityType: entityType, EntityID: id}
	}
	return nil
}

// ColleagueWorkedOnSince reports whether a colleague has acted on the record
// since the audit entry `after`, written at `since`: a human audit row on the
// record itself, on a record a human merged into it, a correction a human
// ruled about it, or a link it is an end of, or a tag or list membership a
// human gave it — archiving the record retires all of them. Tags and lists are
// asked without a time: every caller's entry made the record or brought it
// back, and a human tag on it is work a colleague did on it either way. A human's undo of
// some other change is not work on the record, so a reversal does not count,
// and neither does a row this transaction wrote itself.
//
// "Since" is asked of the audit id as well as the time. A row's occurred_at is
// when its transaction STARTED, so a colleague whose save began before the
// entry and committed after it carries an earlier time; its id is minted at
// the write, so the id still orders it after.
func ColleagueWorkedOnSince(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, since time.Time, after ids.UUID) (bool, error) {
	var worked bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM audit_log a
			 WHERE a.actor_type = 'human' AND (a.occurred_at > $3 OR ($6::uuid IS NOT NULL AND a.id > $6))
			   AND a.occurred_at <> now()
			   AND NOT (coalesce(a.evidence, '{}'::jsonb) ? $4)
			   AND ((a.entity_type = $1 AND a.entity_id = $2)
			        OR (a.entity_type = $1 AND a.after ->> 'merged_into_id' = $2::text)
			        OR (a.entity_type = 'ai_feedback' AND a.after ->> 'subject_type' = $1
			            AND a.after ->> 'subject_id' = $2::text)
			        OR (a.entity_type = $5 AND a.entity_id IN (
			              SELECT r.id FROM relationship r
			               WHERE $2 IN (r.contact_id, r.counterparty_contact_id, r.company_id,
			                            r.counterparty_company_id, r.deal_id, r.project_id)))))
		    OR EXISTS (
			SELECT 1 FROM taggable g
			 WHERE g.entity_type = $1 AND g.entity_id = $2 AND g.assigned_by_kind = 'human')
		    OR EXISTS (
			SELECT 1 FROM list_member m
			 WHERE m.entity_type = $1 AND m.entity_id = $2 AND m.added_by LIKE 'human:%')`,
		entityType, id, since, storekit.EvidenceKeyUndidAuditLog, tableRelationship, entryOrNone(after)).Scan(&worked); err != nil {
		return false, fmt.Errorf("checking whether a colleague worked on this %s: %w", entityType, err)
	}
	return worked, nil
}

// ArchiveDroppedColleagueWork reports whether the newest archive of the record
// deleted a tag or a list membership a human gave it. Asked after an archive in
// its own transaction, it sees exactly what that archive took down — a tag a
// colleague added while the decision was being made included, which a check
// of the live rows can no longer see once the archive has deleted them.
func ArchiveDroppedColleagueWork(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) (bool, error) {
	archive, err := storekit.LatestArchive(ctx, tx, entityType, id)
	if err != nil {
		return false, err
	}
	for _, m := range archive.Cascade.Memberships {
		if strings.HasPrefix(m.AddedBy, "human:") {
			return true, nil
		}
	}
	for _, tag := range archive.Cascade.Tags {
		if tag.AssignedByKind != nil && *tag.AssignedByKind == "human" {
			return true, nil
		}
	}
	return false, nil
}

// entryOrNone is the entry to order by, or none when the caller had only a time.
func entryOrNone(entry ids.UUID) *ids.UUID {
	if entry == ids.Nil {
		return nil
	}
	return &entry
}
