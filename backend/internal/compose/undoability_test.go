// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The refusals that read the row and the image, and nothing else. Each is
// reachable without a database because the branch order asks them before the
// trail is read at all — which is also why an archived record or one the caller
// cannot change costs no query.
//
// The four that DO read the trail — superseded, the erasure boundary,
// already-undone and the value check — are proved in the integration lane
// against real rows, because a test that supplied its own version of those
// answers would prove nothing about production.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// evaluateWithoutTheTrail runs the branches that decide before any query. A nil
// transaction is the assertion: if a branch reached the trail, this panics
// rather than passing on a value nobody supplied.
func evaluateWithoutTheTrail(t *testing.T, e Evaluator, row AuditRow) Undoability {
	t.Helper()
	var noTx pgx.Tx
	answer, err := e.Evaluate(context.Background(), noTx, row, Advisory)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return answer
}

func personRow(before string) AuditRow {
	return AuditRow{
		ID:         ids.NewV7(),
		EntityType: "person",
		EntityID:   ids.NewV7(),
		Action:     "update",
		Before:     json.RawMessage(before),
	}
}

// A verb outside {update, restore} is not reversed by replaying an image.
// archive, promote and merge each have their own verb and their own undo.
func TestAVerbThatIsNotAnImageReplayIsRefused(t *testing.T) {
	row := personRow(`{"full_name":"Greta"}`)
	row.Action = "archive"
	answer := evaluateWithoutTheTrail(t, Evaluator{}, row)
	if answer.Reason != ReasonNotAReplayableVerb {
		t.Errorf("archive: reason = %q, want %q", answer.Reason, ReasonNotAReplayableVerb)
	}
}

// A restore row IS replayable. Admitting it is what makes undoing an undo work,
// and refusing it would leave the reversed entry stuck as already_undone
// forever. Whether such a row survives the REST of the branches is the
// integration lane's question; this holds the verb branch alone, which is the
// one that would silently close that direction off.
func TestARestoreVerbIsItselfReplayable(t *testing.T) {
	if !replayableVerb("restore") {
		t.Error("the restore verb was rejected; undoing an undo is the trail's other direction")
	}
	if replayableVerb("advance_stage") {
		t.Error("advance_stage was admitted; a stage move has its own verb and is not reversed by a field patch")
	}
}

// A type the history screens do not serve is refused by name rather than
// half-served. relationship is a write shape with no history endpoint.
func TestARecordTypeWithNoHistoryScreenIsRefusedByName(t *testing.T) {
	row := personRow(`{"role":"cfo"}`)
	row.EntityType = "relationship"
	answer := evaluateWithoutTheTrail(t, Evaluator{}, row)
	if answer.Reason != ReasonUnsupportedRecordType {
		t.Errorf("relationship: reason = %q, want %q", answer.Reason, ReasonUnsupportedRecordType)
	}
}

// The image is judged AFTER the filter. Each of these is present as an image
// and reduces to nothing a restore could send, so each must answer the same
// refusal — an entry offering to restore nothing is the greyed button with no
// reason this feature exists to avoid.
func TestAnImageThatFiltersToNothingIsRefusedAsNoBeforeImage(t *testing.T) {
	for name, before := range map[string]string{
		"absent":               ``,
		"json null":            `null`,
		"empty object":         `{}`,
		"only derived columns": `{"updated_at":"2026-01-01T00:00:00Z","id":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			answer := evaluateWithoutTheTrail(t, Evaluator{}, personRow(before))
			if answer.Reason != ReasonNoBeforeImage {
				t.Errorf("reason = %q, want %q", answer.Reason, ReasonNoBeforeImage)
			}
		})
	}
}

// An image key the record's update shape cannot spell is NAMED, never dropped.
// A person's update changed a title and an address; the address arrives as
// address_line1…address_country and the shape spells only a structured
// `address`. Quietly restoring the title would put half the change back and
// report success — worse than refusing, because the person reads the
// confirmation and stops looking.
func TestAnImageTheShapeCannotSpellIsRefusedByNamingTheField(t *testing.T) {
	answer := evaluateWithoutTheTrail(t, Evaluator{},
		personRow(`{"title":"CTO","address_city":"Hanoi"}`))
	if answer.Reason != ReasonNotRestorableByThisPath {
		t.Fatalf("reason = %q, want %q", answer.Reason, ReasonNotRestorableByThisPath)
	}
	if !strings.Contains(answer.Detail, "address_city") {
		t.Errorf("the refusal does not name the field it could not spell: %q", answer.Detail)
	}
}

// A restore in a workspace whose records live in an incumbent system is refused
// before it writes. The write-back path records its own verb and its own
// evidence, so the link naming the reversed row is never written — nothing
// would read as undone, and the change would already have happened in two
// systems by the time anyone noticed.
func TestARecordHeldInAnExternalSystemIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	e := Evaluator{ExternallyGoverned: func(context.Context) (bool, error) { return true, nil }}
	answer := evaluateWithoutTheTrail(t, e, personRow(`{"title":"CTO"}`))
	if answer.Reason != ReasonNotRestorableByThisPath {
		t.Errorf("reason = %q, want %q", answer.Reason, ReasonNotRestorableByThisPath)
	}
}

// An archived record's update path refuses on its own terms. Naming it here
// makes the refusal legible instead of a surprise the person reads as a bug.
func TestAnArchivedRecordIsRefusedBeforeTheTrailIsRead(t *testing.T) {
	e := Evaluator{Archived: func(context.Context, pgx.Tx, string, ids.UUID) (bool, error) {
		return true, nil
	}}
	answer := evaluateWithoutTheTrail(t, e, personRow(`{"full_name":"Greta"}`))
	if answer.Reason != ReasonRecordArchived {
		t.Errorf("reason = %q, want %q", answer.Reason, ReasonRecordArchived)
	}
}

// A caller who may read the history but not change the record gets an honest
// button, not a 403 waiting to happen. The row-scope error itself is NOT
// surfaced: it separates "not yours" from "does not exist", which is the
// distinction the row-scope gate keeps hidden.
func TestACallerWhoCannotWriteTheRecordGetsAnHonestButton(t *testing.T) {
	e := Evaluator{Writable: func(context.Context, pgx.Tx, string, ids.UUID) error {
		return errNotYours
	}}
	answer := evaluateWithoutTheTrail(t, e, personRow(`{"full_name":"Greta"}`))
	if answer.Reason != ReasonNotWritableByCaller {
		t.Errorf("reason = %q, want %q", answer.Reason, ReasonNotWritableByCaller)
	}
	if strings.Contains(answer.Detail, errNotYours.Error()) {
		t.Errorf("the refusal leaked the row-scope error: %q", answer.Detail)
	}
}

// The dishonest-success case. activity's update path writes due_at as
// coalesce($n, due_at), so restoring the image's NULL would report success and
// leave the current value standing. The refusal names the field.
func TestRestoringNullIntoACoalesceGuardedColumnIsRefusedByFieldName(t *testing.T) {
	row := AuditRow{
		ID: ids.NewV7(), EntityType: "activity", EntityID: ids.NewV7(),
		Action: "update", Before: json.RawMessage(`{"subject":"Call","due_at":null}`),
	}
	answer := evaluateWithoutTheTrail(t, Evaluator{}, row)
	if answer.Reason != ReasonNullUnwritableByModule {
		t.Fatalf("reason = %q, want %q", answer.Reason, ReasonNullUnwritableByModule)
	}
	if !strings.Contains(answer.Detail, "due_at") {
		t.Errorf("the refusal does not name the field: %q", answer.Detail)
	}
}

// A NULL in a column the update path CAN clear is not this refusal. person
// patches through storekit.Patch, which writes exactly the columns the caller
// supplied, so refusing its nulls would withhold restores that work.
func TestRestoringNullIntoAClearableColumnIsNotRefusedForNullness(t *testing.T) {
	patch := map[string]json.RawMessage{"title": json.RawMessage("null")}
	if unwritable := nullUnwritableFields("person", patch); len(unwritable) > 0 {
		t.Errorf("person's title reported unwritable: %v", unwritable)
	}
}

// A coalesce-guarded column holding a real value restores fine. Only the NULL
// is unwritable, and refusing the whole column would refuse most of the entries
// on an activity's history.
func TestACoalesceGuardedColumnWithAValueIsNotRefused(t *testing.T) {
	patch := map[string]json.RawMessage{"due_at": json.RawMessage(`"2026-01-01T00:00:00Z"`)}
	if unwritable := nullUnwritableFields("activity", patch); len(unwritable) > 0 {
		t.Errorf("a due_at holding a value reported unwritable: %v", unwritable)
	}
}

// Reason is empty exactly when Undoable. A refusal with no reason and an
// undoable answer carrying one are both states the surface cannot render.
func TestAnAnswerCarriesAReasonExactlyWhenItRefuses(t *testing.T) {
	if answer := (undoable()); answer.Reason != "" {
		t.Errorf("an undoable answer carries reason %q", answer.Reason)
	}
	for _, reason := range Reasons {
		if answer := refuse(reason, ""); answer.Undoable {
			t.Errorf("%q produced an undoable answer", reason)
		}
	}
}

var errNotYours = &rowScopeError{}

type rowScopeError struct{}

func (*rowScopeError) Error() string { return "record not found" }
