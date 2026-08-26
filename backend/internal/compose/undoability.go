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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/auditverb"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/datasource"
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
	// ReasonNullUnwritableByModule: the image puts a field back to NULL through
	// a coalesce-guarded column. The write would report success and change
	// nothing, which is worse than a refusal — the person reads the
	// confirmation and stops looking.
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
	ReasonSuperseded,
	ReasonBehindErasureBoundary,
	ReasonAlreadyUndone,
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

// derivedColumns never travel in a restore even when the image carries them.
// They are the write path's own output, not a person's decision, and replaying
// one would state a stamp nobody made.
var derivedColumns = map[string]bool{
	"updated_at": true, "created_at": true, "id": true, "version": true,
}

// namedByTheShapeButNotWrittenByThePatch: keys a record type's update REQUEST
// declares that its update path does not write. The generated shape is the
// contract's, and the module's mapper is narrower than it — a key in the gap is
// accepted, ignored, and answers success, which is the silent-drop failure this
// whole refusal set exists to prevent.
//
// deals is the whole of it today. fx_rate_to_base and fx_rate_date are DERIVED
// from the amount and currency, so a restore that puts those two back re-derives
// them; replaying a stored rate would state a conversion nobody performed.
// status and lost_reason belong to the advance-and-close path, and a field patch
// is not how a deal's lifecycle moves.
//
// Held by TestARestoreLandsEveryFieldItSends
// (backend/internal/compose/recordrestoreshape_integration_test.go), which sets
// every field a record type's shape declares, restores, and reports any that did
// not land — so a key that joins the gap later is named rather than dropped.
var namedByTheShapeButNotWrittenByThePatch = map[string]map[string]bool{
	"deal": {
		"fx_rate_to_base": true, "fx_rate_date": true,
		"status": true, "lost_reason": true,
	},
}

// filterImage reduces a before-image to the patch a restore could send, and
// reports the keys it had to leave behind.
//
// Those two answers travel together because the second is not a detail. A
// person's update changed a title and an address; the address arrives in the
// image as address_line1…address_country and the update shape spells only a
// structured `address`, so a filter that quietly kept the title would put half
// the change back and report success. That is the dishonest success this whole
// refusal set exists to prevent, and it is worse than refusing, because the
// person reads the confirmation and stops looking.
//
// Derived columns and the keys a record type's shape names but its mapper never
// writes are dropped SILENTLY and on purpose: neither was a person's decision,
// and neither is missing from the restore in any sense a reader would care about.
func filterImage(entityType string, before json.RawMessage) (map[string]json.RawMessage, []string, error) {
	var image map[string]json.RawMessage
	if len(before) > 0 {
		if err := json.Unmarshal(before, &image); err != nil {
			return nil, nil, fmt.Errorf("compose: undoability: before-image is not a JSON object: %w", err)
		}
	}
	writable, served := agents.UpdatableFields(datasource.EntityType(entityType))
	if !served {
		return nil, nil, nil
	}
	allowed := make(map[string]bool, len(writable))
	for _, field := range writable {
		allowed[field] = true
	}
	patch := make(map[string]json.RawMessage, len(image))
	var unspellable []string
	for key, value := range image {
		if derivedColumns[key] || namedByTheShapeButNotWrittenByThePatch[entityType][key] {
			continue
		}
		// A cf_* key is a custom field: the catalog decides whether it is still
		// writable, and that is a live-state question the value check owns. The
		// shape check admits it here so the value check can name it.
		if !allowed[key] && !strings.HasPrefix(key, "cf_") {
			unspellable = append(unspellable, key)
			continue
		}
		patch[key] = value
	}
	sort.Strings(unspellable)
	return patch, unspellable, nil
}

// sortedFields spells a patch's keys for a refusal's detail, so the person hears
// which field stopped the restore rather than that something did.
func sortedFields(patch map[string]json.RawMessage) []string {
	out := make([]string, 0, len(patch))
	for key := range patch {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// nullUnwritableFields names the patch keys whose restored value is JSON null in
// a column this record type's update path guards with coalesce. Those writes
// would succeed and change nothing.
func nullUnwritableFields(entityType string, patch map[string]json.RawMessage) []string {
	guarded := CoalesceGuardedColumns(entityType)
	if len(guarded) == 0 {
		return nil
	}
	isGuarded := make(map[string]bool, len(guarded))
	for _, column := range guarded {
		isGuarded[column] = true
	}
	var unwritable []string
	for key, value := range patch {
		if isGuarded[key] && string(value) == "null" {
			unwritable = append(unwritable, key)
		}
	}
	sort.Strings(unwritable)
	return unwritable
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
	if unwritable := nullUnwritableFields(row.EntityType, patch); len(unwritable) > 0 {
		return refuse(ReasonNullUnwritableByModule, strings.Join(unwritable, ", ")), true, nil
	}
	return Undoability{}, false, nil
}

// trailState asks the refusals that read the audit trail, then the value check
// the write path owns.
func (e Evaluator) trailState(ctx context.Context, tx pgx.Tx, row AuditRow, patch map[string]json.RawMessage, mode Mode) (Undoability, error) {
	superseded, err := supersededFieldsTx(ctx, tx, row.EntityType, row.EntityID,
		sortedFields(patch), auditCutoff{OccurredAt: row.OccurredAt, ID: row.ID})
	if err != nil {
		return Undoability{}, err
	}
	if len(superseded) > 0 {
		return refuse(ReasonSuperseded, strings.Join(superseded, ", ")), nil
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
	if e.AlreadyUndone != nil {
		undone, err := e.AlreadyUndone(ctx, tx, row)
		if err != nil {
			return Undoability{}, err
		}
		if undone {
			return refuse(ReasonAlreadyUndone, ""), nil
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
