// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether one audited change can be put back is COMPUTED, never stored. A
// stored flag is a second copy of a question the spine already answers, and it
// goes stale the moment anyone else writes.
//
// The evaluator runs twice. Advisory, on the history read, so the button is
// honest before anyone presses it. Binding, inside the module's own write
// transaction after the row lock, so the write cannot act on a snapshot taken
// before it. The two must agree on the SET of reasons they can return; they may
// differ on timing, and three reasons are only best-effort on the read because
// they depend on state the write path owns.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/auditverb"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// Reason names why one audited change cannot be put back. Every reason is a
// sentence the product says to a person, so each is separately named, separately
// reachable and separately tested — a greyed button with no reason is the shape
// this feature exists to avoid.
//
// undoreasoncensus_test.go holds this set equal to the crm.yaml enum and the
// frontend's strings. One refusal set, three spellings.
type Reason string

const (
	// ReasonNoBeforeImage: the FILTERED image is absent, empty, or has no key
	// the update path could write. Judged after the filter, never before —
	// an image whose only keys are non-writable filters to nothing, and an
	// entry that says "restore" over nothing is a button that does nothing.
	ReasonNoBeforeImage Reason = "no_before_image"
	// ReasonNotAReplayableVerb: the action is outside {update, restore}. A
	// restore row IS replayable — it carries real images by construction, and
	// admitting it is what makes undoing an undo work. archive, promote and
	// merge have their own verbs; a field patch is not how you reverse them.
	ReasonNotAReplayableVerb Reason = "not_a_replayable_verb"
	// ReasonUnsupportedRecordType: the type is outside the six this path
	// serves. relationship is in the write shapes and has no history endpoint,
	// so it is honestly unsupported here rather than silently absent.
	ReasonUnsupportedRecordType Reason = "unsupported_record_type"
	// ReasonSuperseded: a later audit row wrote one of these fields. The
	// product refuses; it never clobbers. Where another person edited in
	// between, the result is ambiguous and saying so IS the behaviour.
	ReasonSuperseded Reason = "superseded"
	// ReasonBehindErasureBoundary: a scrub tombstone is newer than this row, so
	// its images are past an Art. 17 erasure. Restoring from them would write
	// back what an erasure removed.
	ReasonBehindErasureBoundary Reason = "behind_erasure_boundary"
	// ReasonAlreadyUndone: a live restore already reverses this row. It is not
	// terminal — reversing that restore reopens this entry, which is what makes
	// the trail navigable in both directions.
	ReasonAlreadyUndone Reason = "already_undone"
	// ReasonNotRestorableByThisPath: the reversal path cannot put this entry
	// back. Three things reach it: a custom field retired since the change, an
	// image holding keys the record's update shape cannot spell, and a record
	// whose system of record is external.
	//
	// It does NOT cover every value that a module would refuse today — a stage
	// since retired, a close date now in the past, an owner since deactivated.
	// Those still surface as the module's own error, and closing that gap means
	// asking each module what it would accept, which is its own change.
	ReasonNotRestorableByThisPath Reason = "not_restorable_by_this_path"
	// ReasonRecordArchived: the record is archived. Its update path refuses on
	// its own terms; naming it here makes the refusal legible rather than a
	// surprise.
	ReasonRecordArchived Reason = "record_archived"
	// ReasonNullUnwritableByModule: the image puts a field back to NULL, and no
	// update path here can write one. Every field on every update request is an
	// optional pointer, so a JSON null decodes to "not supplied" and is
	// indistinguishable from omitting the field; activity's columns are
	// additionally coalesce-guarded in SQL, which would swallow a null even if
	// one arrived. The write would report success and change nothing, which is
	// worse than a refusal — the person reads the confirmation and stops
	// looking.
	ReasonNullUnwritableByModule Reason = "null_unwritable_by_module"
	// ReasonNotWritableByCaller: the caller may read this history but not
	// change the record, so the button is honest rather than a 403 waiting to
	// happen.
	ReasonNotWritableByCaller Reason = "not_writable_by_caller"
)

// Reasons lists the refusals in the order the branches are asked. It is the
// corpus the contract, the frontend copy and the branch walk are all held
// against, so a reason missing from it is a refusal nothing checks.
//
// Held by: TestEveryReasonABranchReturnsIsListed and
// TestTheContractAdmitsExactlyTheReasonsTheEvaluatorCanReturn
// (backend/undoreasoncensus_test.go)
var Reasons = []Reason{
	ReasonNotAReplayableVerb,
	ReasonUnsupportedRecordType,
	ReasonNoBeforeImage,
	ReasonRecordArchived,
	ReasonNotWritableByCaller,
	ReasonAlreadyUndone,
	ReasonSuperseded,
	ReasonBehindErasureBoundary,
	ReasonNotRestorableByThisPath,
	ReasonNullUnwritableByModule,
}

// Undoability is the answer for one audit row.
type Undoability struct {
	Undoable bool
	// Reason is empty exactly when Undoable.
	Reason Reason
	// Detail is the module's own explanation where that is the better one — the
	// field a refusal names, or the write path's own message. It is never the
	// only thing a caller reads: Reason is what the product renders.
	Detail string
}

func undoable() Undoability { return Undoability{Undoable: true} }

func refuse(reason Reason, detail string) Undoability {
	return Undoability{Reason: reason, Detail: detail}
}

// AuditRow is one history entry, as the evaluator needs it.
type AuditRow struct {
	ID         ids.UUID
	EntityType string
	EntityID   ids.UUID
	Action     string
	Before     json.RawMessage
	// After is what this entry left the fields at. Supersession compares it
	// with what the record holds now, which is what lets several changes be
	// undone in a row.
	After      json.RawMessage
	OccurredAt time.Time
}

// Mode says which of the two evaluations this is. Advisory answers three of the
// reasons best-effort, because they depend on state only the write path holds
// under a lock; Binding answers all of them after taking it.
type Mode int

const (
	Advisory Mode = iota
	Binding
)

// undoableRecordTypes are the six the history screens serve. relationship is a
// write shape without a history endpoint, so a restore of one has nowhere to be
// pressed from and is refused by name rather than half-served.
var undoableRecordTypes = []string{
	"person", "organization", "deal", "lead", "project", "activity",
}

// replayableVerbs are the two an image replay can reverse.
func replayableVerb(action string) bool {
	return auditverb.Verb(action).Valid()
}

func servesRecordType(entityType string) bool {
	for _, served := range undoableRecordTypes {
		if served == entityType {
			return true
		}
	}
	return false
}

// Evaluator answers undoability for one audit row. Its dependencies are the
// live-state readers the refusals need; each is a port so the advisory read and
// the binding write share ONE set of branches rather than two that drift.
type Evaluator struct {
	// Archived reports whether the record is archived.
	Archived func(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) (bool, error)
	// Writable reports whether the caller may change the record. It returns the
	// row-scope error unchanged so a caller that wants to surface it can.
	Writable func(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) error
	// BehindErasure reports whether a scrub tombstone is newer than this row.
	BehindErasure func(ctx context.Context, tx pgx.Tx, row AuditRow) (bool, error)
	// AlreadyUndone reports whether a live restore already reverses this row.
	AlreadyUndone func(ctx context.Context, tx pgx.Tx, row AuditRow) (bool, error)
	// Unwritable names values in the patch the update path could not write
	// today — a retired enum member, a deleted custom field, a departed owner.
	// It is best-effort on the read and authoritative inside the write.
	Unwritable func(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, patch map[string]json.RawMessage) ([]string, error)
	// ExternallyGoverned reports whether this workspace's records live in an
	// incumbent system rather than here. A reversal there is a write-back, and
	// the write-back path records its own verb and its own evidence — so the
	// link naming the reversed row is never written, nothing reads as undone,
	// and the change has already happened in two systems by the time anyone
	// notices. Saying so first is the only honest answer available.
	ExternallyGoverned func(ctx context.Context) (bool, error)
}

// Evaluate answers whether this row can be put back.
//
// The branch order is cheapest and most certain first, so a row failing several
// reports the most useful one. The image is judged AFTER the filter: the raw
// image can be present and still reduce to nothing a restore could send.
func (e Evaluator) Evaluate(ctx context.Context, tx pgx.Tx, row AuditRow, mode Mode) (Undoability, error) {
	if !replayableVerb(row.Action) {
		return refuse(ReasonNotAReplayableVerb, row.Action), nil
	}
	if !servesRecordType(row.EntityType) {
		return refuse(ReasonUnsupportedRecordType, row.EntityType), nil
	}
	if e.ExternallyGoverned != nil {
		external, err := e.ExternallyGoverned(ctx)
		if err != nil {
			return Undoability{}, err
		}
		if external {
			return refuse(ReasonNotRestorableByThisPath,
				"this workspace's records are held in an external system"), nil
		}
	}
	patch, unspellable, err := filterImage(row.EntityType, row.Before)
	if err != nil {
		return Undoability{}, err
	}
	if len(patch) == 0 && len(unspellable) == 0 {
		return refuse(ReasonNoBeforeImage, ""), nil
	}
	// An entry that changed nothing has nothing to put back. Both images are
	// normalised jsonb by the time they are written, so this compares what the
	// store actually recorded rather than the Go values it recorded them from —
	// which is why the store could not tell and this can.
	if changed, err := recordsAChange(row.Before, row.After); err != nil {
		return Undoability{}, err
	} else if !changed {
		return refuse(ReasonNoBeforeImage, "this change left every field as it was"), nil
	}
	// Judged before the patch is: an entry that can only be put back in part
	// must refuse, not put back the part it can.
	if len(unspellable) > 0 {
		return refuse(ReasonNotRestorableByThisPath, strings.Join(unspellable, ", ")), nil
	}
	if answer, decided, err := e.liveState(ctx, tx, row, patch); err != nil || decided {
		return answer, err
	}
	return e.trailState(ctx, tx, row, patch, mode)
}

// liveState asks the refusals that read the record itself. A record that is
// archived or not the caller's to change is refused before the trail is read at
// all, because neither answer can be improved by reading it.
func (e Evaluator) liveState(ctx context.Context, tx pgx.Tx, row AuditRow, patch map[string]json.RawMessage) (Undoability, bool, error) {
	if e.Archived != nil {
		archived, err := e.Archived(ctx, tx, row.EntityType, row.EntityID)
		if err != nil {
			return Undoability{}, false, err
		}
		if archived {
			return refuse(ReasonRecordArchived, ""), true, nil
		}
	}
	if e.Writable != nil {
		if err := e.Writable(ctx, tx, row.EntityType, row.EntityID); err != nil {
			// The row-scope error is not surfaced: it distinguishes "not yours"
			// from "does not exist", and that distinction is what the row-scope
			// gate keeps hidden.
			return refuse(ReasonNotWritableByCaller, ""), true, nil
		}
	}
	if _, _, unclearable := splitNulls(row.EntityType, patch); len(unclearable) > 0 {
		return refuse(ReasonNullUnwritableByModule, strings.Join(unclearable, ", ")), true, nil
	}
	return Undoability{}, false, nil
}

// trailState asks the refusals that read the audit trail, then the value check
// the write path owns.
func (e Evaluator) trailState(ctx context.Context, tx pgx.Tx, row AuditRow, patch map[string]json.RawMessage, mode Mode) (Undoability, error) {
	// Asked before supersession: an entry that has been put back should say so.
	// Its own reversal moved the fields away from what it left them at, so
	// supersession would otherwise answer first and less usefully.
	if e.AlreadyUndone != nil {
		undone, err := e.AlreadyUndone(ctx, tx, row)
		if err != nil {
			return Undoability{}, err
		}
		if undone {
			return refuse(ReasonAlreadyUndone, ""), nil
		}
	}
	moved, err := fieldsThatMovedSince(ctx, tx, row.EntityType, row.EntityID, row.After)
	if err != nil {
		return Undoability{}, err
	}
	if len(moved) > 0 {
		return refuse(ReasonSuperseded, strings.Join(moved, ", ")), nil
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
	if e.Unwritable != nil {
		unwritable, err := e.Unwritable(ctx, tx, row.EntityType, row.EntityID, patch)
		if err != nil {
			// Advisory mode answers this one best-effort: the button is as
			// honest as a read can make it, and the write is what binds. A read
			// that failed here must not hide the whole page.
			if mode == Advisory {
				return undoable(), nil
			}
			return Undoability{}, err
		}
		if len(unwritable) > 0 {
			return refuse(ReasonNotRestorableByThisPath, strings.Join(unwritable, ", ")), nil
		}
	}
	return undoable(), nil
}
