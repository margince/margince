// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Judging an entry whose undo is a module's own verb rather than a field
// replay.
//
// A create is undone by archiving the record, an archive by un-archiving it, a
// lead's promotion by demoting it, and a machine fill of a contact's profile
// fields by clearing them again. None of them has a before-image an update could
// send, so each is its own branch, the way a link's change is (edgeundoability.go).
// They share the record path's guards and its reason vocabulary: the caller may
// change the record, the entry is not already undone, nothing has moved since,
// and the entry is not behind an erasure.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// inverse names the module verb that undoes one entry.
type inverse int

const (
	inverseNone inverse = iota
	// inverseArchive undoes a create.
	inverseArchive
	// inverseUnarchive undoes an archive.
	inverseUnarchive
	// inverseDemote undoes a lead's promotion.
	inverseDemote
	// inverseRetractFill undoes a machine fill of a contact's profile fields.
	inverseRetractFill
	// inverseRearchive redoes an archive: it undoes the un-archive that
	// reversed it.
	inverseRearchive
)

// The audit verbs an inverse answers to.
const (
	actionCreate  = "create"
	actionArchive = "archive"
	actionPromote = "promote"
)

// archivedByUndo are the record types whose create an undo archives. A lead
// has no archive. A project's archive retires its stakeholder links in a
// transaction of its own, where a link a colleague added after the decision
// cannot be seen; until it can be archived on the caller's transaction, its
// create stays a verb no undo reverses.
//
//nolint:goconst // record types read as data, listed where the branch is decided
var archivedByUndo = map[string]bool{
	"contact": true, "company": true, "deal": true, entityTypeActivity: true,
}

// unarchivedByUndo are the record types with an un-archive (contacts'
// RestoreContactTx and RestoreCompanyTx, deals' RestoreDealTx). Another type's
// archive stays a verb no undo reverses.
var unarchivedByUndo = map[string]bool{"contact": true, "company": true, entityTypeDeal: true}

// inverseOf answers which module verb undoes this entry, or inverseNone where
// the entry is a field replay or nothing undoes it.
func inverseOf(row AuditRow) inverse {
	switch {
	case row.Action == actionCreate && archivedByUndo[row.EntityType]:
		return inverseArchive
	case row.Action == actionArchive && unarchivedByUndo[row.EntityType]:
		return inverseUnarchive
	case row.Action == actionPromote && row.EntityType == entityTypeLead:
		return inverseDemote
	case row.Action == actionRestore && unarchivedByUndo[row.EntityType] && restoresAnArchive(row):
		return inverseRearchive
	case row.EntityType == entityTypeContact:
		if _, fill := fillOf(row); fill {
			return inverseRetractFill
		}
	}
	return inverseNone
}

// actionRestore is the verb an un-archive and a field restore both write.
const actionRestore = "restore"

// restoresAnArchive reports whether a restore row is an un-archive, which names
// the archive it brought the record back from.
func restoresAnArchive(row AuditRow) bool {
	var evidence map[string]json.RawMessage
	if json.Unmarshal(row.Evidence, &evidence) != nil {
		return false
	}
	_, unarchived := evidence[storekit.EvidenceKeyRestoresArchive]
	return unarchived
}

// fillOf asks the contacts module whether this entry is a fill it can take back.
func fillOf(row AuditRow) (contacts.FillRetraction, bool) {
	if row.Action != auditActionUpdate || len(row.Before) == 0 || len(row.After) == 0 || len(row.Evidence) == 0 {
		return contacts.FillRetraction{}, false
	}
	fill, ok := contacts.FillOf(row.Before, row.After, row.Evidence, row.OccurredAt)
	fill.Entry = row.ID
	return fill, ok
}

// The record kinds an inverse is about.
const (
	entityTypeLead    = "lead"
	entityTypeContact = "contact"
	auditActionUpdate = "update"
)

// undoGrantFor is the object grant the undo of this entry asks: an archive and
// an un-archive ask delete, as the modules' own archive and restore do; every
// other undo is a write and asks update.
func undoGrantFor(row AuditRow) principal.Action {
	switch inverseOf(row) {
	case inverseArchive, inverseUnarchive, inverseRearchive:
		return principal.ActionDelete
	case inverseNone, inverseDemote, inverseRetractFill:
	}
	return principal.ActionUpdate
}

// evaluateInverse answers whether an entry a module verb undoes can be undone
// now. Same order as the record path: who may act first, then whether it is
// already undone, then whether anything has moved since, then the erasure
// boundary.
func (e Evaluator) evaluateInverse(ctx context.Context, tx pgx.Tx, row AuditRow, kind inverse) (Undoability, error) {
	// The object grant the module verb asks at its own door: an archive asks
	// delete, which a seat that may edit a record often does not hold.
	if err := auth.Require(ctx, row.EntityType, undoGrantFor(row)); err != nil {
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			return Undoability{}, err
		}
		return refuse(ReasonNotWritableByCaller, ""), nil
	}
	if e.Writable != nil {
		if err := e.Writable(ctx, tx, row.EntityType, row.EntityID); err != nil {
			if !isWriteScopeRefusal(err) {
				return Undoability{}, err
			}
			return refuse(ReasonNotWritableByCaller, ""), nil
		}
	}
	if e.AlreadyUndone != nil {
		undone, err := e.AlreadyUndone(ctx, tx, row)
		if err != nil {
			return Undoability{}, err
		}
		if undone {
			return refuse(ReasonAlreadyUndone, ""), nil
		}
	}
	if answer, decided, err := e.inverseState(ctx, tx, row, kind); err != nil || decided {
		return answer, err
	}
	if e.BehindErasure != nil {
		behind, err := e.BehindErasure(ctx, tx, row)
		if err != nil {
			return Undoability{}, err
		}
		if behind {
			return refuse(ReasonBehindErasureBoundary, ""), nil
		}
	}
	return undoable(), nil
}

// inverseState asks what the record says now: whether the state this entry
// left is still the state an undo would reverse.
func (e Evaluator) inverseState(ctx context.Context, tx pgx.Tx, row AuditRow, kind inverse) (Undoability, bool, error) {
	switch kind {
	case inverseArchive:
		return e.createStands(ctx, tx, row)
	case inverseUnarchive:
		return e.archiveStands(ctx, tx, row)
	case inverseDemote:
		return e.promotionStands(ctx, tx, row)
	case inverseRetractFill:
		return e.fillStands(ctx, tx, row)
	case inverseRearchive:
		// Archiving again is the same write as undoing a create, with the
		// same test: the record is live, and nobody has worked on it since it
		// came back.
		return e.createStands(ctx, tx, row)
	case inverseNone:
	}
	return Undoability{}, false, nil
}

// promotionStands refuses the undo of a promotion once the lead has moved on,
// or once a colleague has worked on the contact the promotion created: the
// demotion archives that contact. DemoteLead asks the same question again,
// through the same contacts.ColleagueWorkedOnSince, inside its own transaction.
func (e Evaluator) promotionStands(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, bool, error) {
	// The lead's current promotion must be this one: demoted by hand and
	// promoted again into the same contact, its fields read the same while
	// the demotion would reverse the later promotion.
	var current ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM audit_log
		 WHERE entity_type = $1 AND entity_id = $2 AND action = $3
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		entityTypeLead, row.EntityID, actionPromote).Scan(&current); err != nil {
		return Undoability{}, false, err
	}
	if current != row.ID {
		return refuse(ReasonSuperseded, "the lead was promoted again since"), true, nil
	}
	moved, err := fieldsThatMovedSince(ctx, tx, row)
	if err != nil {
		return Undoability{}, false, err
	}
	if len(moved) > 0 {
		return refuse(ReasonSuperseded, strings.Join(moved, ", ")), true, nil
	}
	var promoted struct {
		Contact *ids.UUID `json:"promoted_contact_id"`
		Outcome string    `json:"dedupe_outcome"`
	}
	if err := json.Unmarshal(row.After, &promoted); err != nil {
		return Undoability{}, false, fmt.Errorf("compose: read the promotion's outcome: %w", err)
	}
	if promoted.Contact == nil {
		return Undoability{}, false, nil
	}
	// The demotion writes the contact as well as the lead, and asks for both.
	if answer, refused, err := e.contactWritable(ctx, tx, *promoted.Contact); err != nil || refused {
		return answer, refused, err
	}
	if promoted.Outcome != promotionCreatedContact {
		return Undoability{}, false, nil
	}
	touched, err := contacts.ColleagueWorkedOnSince(ctx, tx, entityTypeContact, *promoted.Contact, row.OccurredAt, row.ID)
	if err != nil {
		return Undoability{}, false, err
	}
	if touched {
		return refuse(ReasonSuperseded, "the contact it created was changed by a colleague since"), true, nil
	}
	return Undoability{}, false, nil
}

// contactWritable refuses when the caller may not change the promoted contact.
func (e Evaluator) contactWritable(ctx context.Context, tx pgx.Tx, contact ids.UUID) (Undoability, bool, error) {
	if err := auth.Require(ctx, entityTypeContact, principal.ActionUpdate); err != nil {
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			return Undoability{}, false, err
		}
		return refuse(ReasonNotWritableByCaller, ""), true, nil
	}
	if e.Writable == nil {
		return Undoability{}, false, nil
	}
	if err := e.Writable(ctx, tx, entityTypeContact, contact); err != nil {
		if !isWriteScopeRefusal(err) {
			return Undoability{}, false, err
		}
		return refuse(ReasonNotWritableByCaller, ""), true, nil
	}
	return Undoability{}, false, nil
}

// promotionCreatedContact is the promote row's dedupe_outcome when the
// promotion created a new contact rather than merging into one.
const promotionCreatedContact = "created"

// createStands refuses the undo of a create once the record is archived, or
// once a colleague has acted on it since: archiving it then would take their
// work out of view with the machine's. A colleague's own undo of some other
// change is not their work on the record, so a reversal does not count.
func (e Evaluator) createStands(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, bool, error) {
	if answer, archived, err := e.archivedRefusal(ctx, tx, row); err != nil || archived {
		return answer, archived, err
	}
	if row.EntityType == entityTypeActivity {
		// A meeting with a live calendar invitation is changed through the
		// invitation; archiving it here would hide a meeting still on
		// somebody's calendar, which the activity archive refuses too.
		var invited bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM meeting_invitation WHERE activity_id = $1 AND status <> 'canceled')`,
			row.EntityID).Scan(&invited); err != nil {
			return Undoability{}, false, err
		}
		if invited {
			return refuse(ReasonNotAReplayableVerb, "a meeting with a calendar invitation"), true, nil
		}
	}
	touched, err := contacts.ColleagueWorkedOnSince(ctx, tx, row.EntityType, row.EntityID, row.OccurredAt, row.ID)
	if err != nil {
		return Undoability{}, false, err
	}
	if touched {
		return refuse(ReasonSuperseded, "changed by a colleague since it was created"), true, nil
	}
	return Undoability{}, false, nil
}

// archiveStands refuses the undo of an archive unless the record is still
// archived by THIS entry: un-archived since, or archived again later, the
// un-archive would reverse a different archive than the one on screen.
func (e Evaluator) archiveStands(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, bool, error) {
	if e.Archived != nil {
		archived, err := e.Archived(ctx, tx, row.EntityType, row.EntityID)
		if err != nil {
			return Undoability{}, false, err
		}
		if !archived {
			return refuse(ReasonSuperseded, "archived_at"), true, nil
		}
	}
	latest, err := storekit.LatestArchive(ctx, tx, row.EntityType, row.EntityID)
	if err != nil {
		return Undoability{}, false, err
	}
	if latest.AuditID != row.ID {
		return refuse(ReasonSuperseded, "archived_at"), true, nil
	}
	// A record merged away since is refused by the un-archive itself, which
	// reads the merge under its own lock (inverseWriteRefusal names it).
	return Undoability{}, false, nil
}

// fillStands refuses the undo of a fill the record has moved on from, and of
// one that replaced a value rather than filling a blank.
func (e Evaluator) fillStands(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, bool, error) {
	if answer, archived, err := e.archivedRefusal(ctx, tx, row); err != nil || archived {
		return answer, archived, err
	}
	fill, _ := fillOf(row)
	if len(fill.Fields) == 0 {
		return refuse(ReasonNotRestorableByThisPath, "the statement only confirmed what the record showed"), true, nil
	}
	err := contacts.JudgeFillRetraction(ctx, tx, ids.From[ids.ContactKind](row.EntityID), fill)
	var refusal *contacts.FillRetractionRefusal
	if errors.As(err, &refusal) {
		return fillRefusal(refusal), true, nil
	}
	return Undoability{}, false, err
}

// fillRefusal renders the module's refusal in the reason vocabulary. A field
// somebody changed since is superseded; a field the fill replaced rather than
// filled is not this path's to put back.
func fillRefusal(refusal *contacts.FillRetractionRefusal) Undoability {
	if len(refusal.Moved) > 0 {
		return refuse(ReasonSuperseded, strings.Join(refusal.Moved, ", "))
	}
	return refuse(ReasonNotRestorableByThisPath, strings.Join(refusal.Replaced, ", "))
}

// archivedRefusal refuses an entry on a record that is archived now.
func (e Evaluator) archivedRefusal(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, bool, error) {
	if e.Archived == nil {
		return Undoability{}, false, nil
	}
	archived, err := e.Archived(ctx, tx, row.EntityType, row.EntityID)
	if err != nil || !archived {
		return Undoability{}, false, err
	}
	return refuse(ReasonRecordArchived, ""), true, nil
}
