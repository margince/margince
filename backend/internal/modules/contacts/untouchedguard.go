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
	// keepWhileReferenced refuses an archive of a company a live deal or a
	// live contact's employment still names.
	keepWhileReferenced bool
	// releaseSourceKey clears a retired lead's (source_system, source_id) so
	// the key can be claimed again.
	releaseSourceKey bool
}

// ReleasingSourceKey makes a lead's retirement free its source key.
// The unique index on that key covers retired rows. Without this, a retired
// lead replays when the same file is imported again.
func ReleasingSourceKey() WriteOption {
	return func(o *writeOptions) { o.releaseSourceKey = true }
}

// NotTouchedByHumanSince refuses the write when a user or their agent has acted
// on the record since `at`. Acting is any such audit row on the entity, not
// only an update. A narrower filter would reverse the act of somebody who
// archived an imported row themselves.
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

// KeepWhileReferenced refuses a company's archive while a live deal names it
// as its company or its partner, or a live contact is employed by it. Without
// it the archive would leave those records pointing at an archived company.
func KeepWhileReferenced() WriteOption {
	return func(o *writeOptions) { o.keepWhileReferenced = true }
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
			"This %s has no version, so a change conditioned on version %d cannot be made to it. "+
				"Nothing was changed. Make the change without a version condition.",
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

// Is makes the refusal a conflict for errors.Is, so an undo records the row as
// one it left alone instead of stopping.
func (e *HumanTouchedError) Is(target error) bool { return target == apperrors.ErrConflict }

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
	touched, err := storekit.HumanActedSince(ctx, tx, entityType, id, *opts.untouchedSince)
	if err != nil {
		return err
	}
	if touched {
		return &HumanTouchedError{EntityType: entityType, EntityID: id}
	}
	return nil
}

// ColleagueWorkedOnSince reports whether a colleague has acted on the record
// since the audit entry `after`, written at `since`. A colleague is a user or
// their agent. Their work is an audit row on the record or on a record merged
// into it. A correction ruled about it counts, and so does a link it is an end
// of. A tag or list
// membership they gave it counts too. Archiving the record retires all of them.
// Tags and lists are asked without a time, since every caller's entry made the
// record or brought it back. A colleague's undo of some other change is not work
// on the record. Neither is a row this transaction wrote itself. A tag names the user
// behind its agent and no passport, so a named user is what counts there.
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
			 WHERE (a.occurred_at > $3 OR ($6::uuid IS NOT NULL AND a.id > $6))
			   AND a.occurred_at <> now()
			   AND NOT (coalesce(a.evidence, '{}'::jsonb) ? $4)
			   AND ((a.entity_type = $1 AND a.entity_id = $2)
			        OR (a.entity_type = $1 AND a.after ->> 'merged_into_id' = $2::text)
			        OR (a.entity_type = 'ai_feedback' AND a.after ->> 'subject_type' = $1
			            AND a.after ->> 'subject_id' = $2::text)
			        OR (a.entity_type = $5 AND a.entity_id IN (
			              SELECT r.id FROM relationship r
			               WHERE $2 IN (r.contact_id, r.counterparty_contact_id, r.company_id,
			                            r.counterparty_company_id, r.deal_id, r.project_id))))
			   AND `+storekit.AuditForAUser("a")+`)
		    OR EXISTS (
			SELECT 1 FROM taggable g
			 WHERE g.entity_type = $1 AND g.entity_id = $2 AND (g.assigned_by_kind = 'human' OR (g.assigned_by_kind = 'agent' AND g.assigned_by IS NOT NULL)))
		    OR EXISTS (
			SELECT 1 FROM list_member m
			 WHERE m.entity_type = $1 AND m.entity_id = $2 AND `+storekit.StampForAUserSQL("m.added_by")+`)`,
		entityType, id, since, storekit.EvidenceKeyUndidAuditLog, tableRelationship, entryOrNone(after)).Scan(&worked); err != nil {
		return false, fmt.Errorf("checking whether a colleague worked on this %s: %w", entityType, err)
	}
	return worked, nil
}

// ArchiveDroppedColleagueWork reports whether the newest archive of the record
// deleted a tag or a list membership a user or their agent gave it. Asked after
// an archive in its own transaction, it sees what that archive took
// down. That includes a tag added while the decision was being made, which a
// check of the live rows can no longer see.
func ArchiveDroppedColleagueWork(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) (bool, error) {
	archive, err := storekit.LatestArchive(ctx, tx, entityType, id)
	if err != nil {
		return false, err
	}
	for _, m := range archive.Cascade.Memberships {
		if storekit.StampIsForAUser(m.AddedBy) {
			return true, nil
		}
	}
	for _, tag := range archive.Cascade.Tags {
		if tag.AssignedByKind != nil && (*tag.AssignedByKind == "human" || (*tag.AssignedByKind == "agent" && tag.AssignedBy != nil)) {
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
