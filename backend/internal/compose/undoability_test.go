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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// personRow is one audited person update. The after image differs from the
// before by construction: an entry whose images match changed nothing, which is
// its own refusal, and a fixture that tripped it would test that branch instead
// of the one each case names.
func personRow(before string) AuditRow {
	return AuditRow{
		ID:         ids.NewV7(),
		EntityType: "person",
		EntityID:   ids.NewV7(),
		Action:     "update",
		Before:     json.RawMessage(before),
		After:      json.RawMessage(`{"full_name":"After The Change"}`),
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
// half-served. A custom field is audited and has no history screen of its own.
func TestARecordTypeWithNoHistoryScreenIsRefusedByName(t *testing.T) {
	row := personRow(`{"role":"cfo"}`)
	row.EntityType = "custom_field"
	answer := evaluateWithoutTheTrail(t, Evaluator{}, row)
	if answer.Reason != ReasonUnsupportedRecordType {
		t.Errorf("custom_field: reason = %q, want %q", answer.Reason, ReasonUnsupportedRecordType)
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
// Quietly restoring the title beside it would put half the change back and
// report success — worse than refusing, because the person reads the
// confirmation and stops looking.
//
// `consent_status` is the durable example: consent is deliberately NOT mutable
// through a person update (A22/ADR-0011 — it moves only through the consent
// endpoint, which writes an append-only proof row), so no widening of
// UpdatePersonRequest will ever make this key spellable.
//
// Two fields that USED to be examples are not any more, and both stopped being
// so by being fixed rather than by this rule weakening. The address columns
// fold into the structured `address` the shape declares, which is what makes an
// address edit reversible at all; and `emails` is now on the update request,
// because a bounced address needed a way to be corrected.
func TestAnImageTheShapeCannotSpellIsRefusedByNamingTheField(t *testing.T) {
	answer := evaluateWithoutTheTrail(t, Evaluator{},
		personRow(`{"title":"CTO","consent_status":"granted"}`))
	if answer.Reason != ReasonNotRestorableByThisPath {
		t.Fatalf("reason = %q, want %q", answer.Reason, ReasonNotRestorableByThisPath)
	}
	if !strings.Contains(answer.Detail, "consent_status") {
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

// A field the record type CAN clear is put back to nothing rather than refused.
// This is the common case a person reaches for undo on: they filled a field in
// by mistake and want it empty again.
func TestAFieldTheRecordTypeCanClearIsClearedRatherThanRefused(t *testing.T) {
	patch := map[string]json.RawMessage{
		"title":     json.RawMessage("null"),
		"full_name": json.RawMessage(`"Greta"`),
	}
	values, cleared, unclearable := splitNulls("person", patch)
	if len(unclearable) > 0 {
		t.Fatalf("person cannot clear %v; a title it filled in is not undoable", unclearable)
	}
	if len(cleared) != 1 || cleared[0] != "title" {
		t.Errorf("cleared = %v, want [title]", cleared)
	}
	if _, sent := values["title"]; sent {
		t.Error("the null travelled in the patch; it decodes to \"not supplied\" and writes nothing")
	}
	if string(values["full_name"]) != `"Greta"` {
		t.Errorf("the value half lost full_name: %v", values)
	}
}

// A field the record type CANNOT clear is refused by name. activity writes
// every column as coalesce($n, col), so no argument can clear one.
func TestAFieldTheRecordTypeCannotClearIsRefusedByName(t *testing.T) {
	answer := evaluateWithoutTheTrail(t, Evaluator{}, AuditRow{
		ID: ids.NewV7(), EntityType: "activity", EntityID: ids.NewV7(),
		Action: "update", Before: json.RawMessage(`{"subject":"Call","due_at":null}`),
		After: json.RawMessage(`{"subject":"Call Greta","due_at":"2026-09-01T10:00:00Z"}`),
	})
	if answer.Reason != ReasonNullUnwritableByModule {
		t.Fatalf("reason = %q, want %q", answer.Reason, ReasonNullUnwritableByModule)
	}
	if !strings.Contains(answer.Detail, "due_at") {
		t.Errorf("the refusal does not name the field: %q", answer.Detail)
	}
}

// A field holding a real value is never a clear.
func TestAFieldHoldingAValueIsNotCleared(t *testing.T) {
	_, cleared, _ := splitNulls("activity",
		map[string]json.RawMessage{"due_at": json.RawMessage(`"2026-01-01T00:00:00Z"`)})
	if len(cleared) > 0 {
		t.Errorf("a due_at holding a value was reported as a clear: %v", cleared)
	}
}

// Reason is empty exactly when Undoable. A refusal with no reason and an
// undoable answer carrying one are both states the surface cannot render.
func TestAnAnswerCarriesAReasonExactlyWhenItRefuses(t *testing.T) {
	if answer := undoable(); answer.Reason != "" {
		t.Errorf("an undoable answer carries reason %q", answer.Reason)
	}
	for _, reason := range Reasons {
		if answer := refuse(reason, ""); answer.Undoable {
			t.Errorf("%q produced an undoable answer", reason)
		}
	}
}

// The row-scope refusal as PRODUCTION spells it. An error of the double's own
// invention would satisfy the branch while telling nothing about the sentinel
// auth.EnsureWritable actually returns — and the evaluator now has to tell that
// refusal apart from a database fault.
var errNotYours = fmt.Errorf("record not found: %w", apperrors.ErrNotFound)

// A fault, not a refusal: the port queries, so it can fail for reasons that
// have nothing to do with who is asking.
var errPortFailed = fmt.Errorf("connection reset")

// A restore button is drawn from a check that may not have run. Reporting a
// database fault as "you may not change this record" tells the person a retry
// is pointless when a retry is the entire answer, and on the write path it
// becomes a 409 whose code says the same.
func TestAFailedWritabilityCheckIsAFaultAndNotARefusal(t *testing.T) {
	e := Evaluator{Writable: func(context.Context, pgx.Tx, string, ids.UUID) error {
		return errPortFailed
	}}
	_, err := e.Evaluate(context.Background(), nil, personRow(`{"full_name":"Greta"}`), Binding)
	if !errors.Is(err, errPortFailed) {
		t.Errorf("err = %v, want the port's own failure to reach the caller", err)
	}
}

// An address arrives in the image one column at a time and is written back as
// one nested object, because that is the only shape the update path accepts.
// Without the fold every address key reads as unspellable and an edit that
// touched an address becomes permanently un-undoable.
func TestAnAddressIsFoldedIntoTheFieldTheUpdatePathAccepts(t *testing.T) {
	patch, unspellable, err := filterImage("organization",
		json.RawMessage(`{"address_city":"Hanoi","address_line1":null,"display_name":"Acme"}`))
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(unspellable) > 0 {
		t.Fatalf("address columns reported unspellable: %v", unspellable)
	}
	folded, present := patch["address"]
	if !present {
		t.Fatal("no address in the patch; the columns were dropped rather than folded")
	}
	var address map[string]any
	if err := json.Unmarshal(folded, &address); err != nil {
		t.Fatalf("the folded address is not an object: %v", err)
	}
	if address["city"] != "Hanoi" {
		t.Errorf("address.city = %v, want Hanoi", address["city"])
	}
	// A null INSIDE a supplied object is a value the update path can write; a
	// bare address_line1 null would have been indistinguishable from absent.
	if line1, held := address["line1"]; !held || line1 != nil {
		t.Errorf("address.line1 = %v (held=%v), want an explicit null", line1, held)
	}
	if _, leaked := patch["address_city"]; leaked {
		t.Error("the raw column travelled beside the folded object")
	}
}

// The fold does not fire for a record type whose update shape has no address.
func TestAnAddressIsNotFoldedForAShapeThatCannotTakeOne(t *testing.T) {
	_, unspellable, err := filterImage("deal", json.RawMessage(`{"address_city":"Hanoi"}`))
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(unspellable) != 1 || unspellable[0] != "address_city" {
		t.Errorf("unspellable = %v, want [address_city] named rather than folded", unspellable)
	}
}

// A null on a field the record holds as a whole object becomes an EMPTY object,
// which is how "there was none" is said. A bare null would decode to "not
// supplied" and the restore would report success having changed nothing.
func TestANullObjectFieldIsRestoredAsAnEmptyObject(t *testing.T) {
	patch, unspellable, err := filterImage("person", json.RawMessage(`{"social":null}`))
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(unspellable) > 0 {
		t.Fatalf("social reported unspellable: %v", unspellable)
	}
	if string(patch["social"]) != `{}` {
		t.Errorf("social = %s, want an empty object", patch["social"])
	}
	// And so it is not a clear the module has to refuse.
	if _, _, unclearable := splitNulls("person", patch); len(unclearable) > 0 {
		t.Errorf("social still reads as an unclearable null: %v", unclearable)
	}
}

// Only the derived stamps are excluded from comparison here. WHICH keys are
// comparable is decided by the row itself — a field kept in its own table is
// absent from the row's jsonb and the query skips it — so this holds the one
// exclusion that is a judgement rather than a fact about the schema.
func TestAStampIsNotComparedForSupersession(t *testing.T) {
	asked, err := coupledImage("person", json.RawMessage(`{"updated_at":"2026-01-01T00:00:00Z","title":"CTO"}`))
	if err != nil {
		t.Fatalf("narrow the image: %v", err)
	}
	var compared map[string]json.RawMessage
	if err := json.Unmarshal(asked, &compared); err != nil {
		t.Fatalf("the narrowed image is not an object: %v", err)
	}
	if _, judged := compared["updated_at"]; judged {
		t.Error("a stamp was compared; the write path set it, not a person")
	}
	if _, judged := compared["title"]; !judged {
		t.Error("title was not compared; it is a person's decision and must be judged")
	}
}

// An entry whose images are the same changed nothing, so there is nothing to
// put back. A store that assigns a column the value it already holds records
// such a row — it compares a *string against a string and cannot tell — and
// offering a button for it is a button that does nothing.
func TestAnEntryThatChangedNothingHasNothingToPutBack(t *testing.T) {
	row := personRow(`{"title":"CTO"}`)
	row.After = json.RawMessage(`{"title":"CTO"}`)
	answer := evaluateWithoutTheTrail(t, Evaluator{}, row)
	if answer.Reason != ReasonNoBeforeImage {
		t.Fatalf("reason = %q, want %q", answer.Reason, ReasonNoBeforeImage)
	}
	if answer.Detail == "" {
		t.Error("the refusal does not say the entry left every field as it was")
	}
}

// A derived stamp moving is not a change worth reversing on its own: the write
// path set it, not a person.
func TestAnEntryWhoseOnlyDifferenceIsAStampHasNothingToPutBack(t *testing.T) {
	row := personRow(`{"title":"CTO","updated_at":"2026-01-01T00:00:00Z"}`)
	row.After = json.RawMessage(`{"title":"CTO","updated_at":"2026-02-02T00:00:00Z"}`)
	if answer := evaluateWithoutTheTrail(t, Evaluator{}, row); answer.Reason != ReasonNoBeforeImage {
		t.Errorf("reason = %q, want %q", answer.Reason, ReasonNoBeforeImage)
	}
}

// A real change is still a change — the check must not refuse everything. Held
// on the function rather than the evaluator, because an entry that passes this
// branch goes on to read the trail.
func TestARealChangeIsRecognisedAsOne(t *testing.T) {
	changed, err := recordsAChange(
		json.RawMessage(`{"title":"CTO"}`), json.RawMessage(`{"title":"CEO"}`),
	)
	if err != nil {
		t.Fatalf("compare the images: %v", err)
	}
	if !changed {
		t.Error("a title moving from CTO to CEO was read as no change")
	}
	// A field appearing for the first time is a change too.
	changed, err = recordsAChange(json.RawMessage(`{"title":null}`), json.RawMessage(`{"title":"CTO"}`))
	if err != nil {
		t.Fatalf("compare the images: %v", err)
	}
	if !changed {
		t.Error("a field filled in for the first time was read as no change")
	}
}

// Setting a custom field from empty cannot be undone, and the refusal is the
// point rather than a shortfall. A cf_* value travels in the request body's
// additionalProperties, where storekit.SQLValue converts it per catalog type
// and DROPS what does not match — a JSON null matches no type, so the write
// would report success and leave the value standing.
//
// Refusing says so. Clearing a custom field is a capability of the platform's
// custom-column contract, which every module's write shares; until it exists,
// this test is what keeps the silent drop from being reintroduced as a
// convenience.
func TestSettingACustomFieldFromEmptyIsRefusedRatherThanSilentlyDropped(t *testing.T) {
	patch := map[string]json.RawMessage{
		"cf_referral_code": json.RawMessage("null"),
		"title":            json.RawMessage(`"CTO"`),
	}
	values, cleared, unclearable := splitNulls("person", patch)
	if len(unclearable) != 1 || unclearable[0] != "cf_referral_code" {
		t.Errorf("unclearable = %v, want the custom field named", unclearable)
	}
	if _, sent := values["cf_referral_code"]; sent {
		t.Error("the null travelled in the patch, where the module drops it and answers success")
	}
	for _, field := range cleared {
		if field == "cf_referral_code" {
			t.Error("the custom field was asked to be cleared, which no module's write can do")
		}
	}
}

// A provenance stamp is dropped for the record type that STAMPS it, and never
// for one that treats the same word as a field.
//
// `source` is the case this exists for. An organization's lifecycle move writes
// it as machine provenance, so a restore must not replay it. A LEAD's source is
// a value a rep types and can edit, and dropping it would leave the tampered
// value in place while reporting the undo a success — the silent-drop failure
// the whole refusal set exists to prevent.
func TestAProvenanceStampIsDroppedOnlyForTheRecordThatStampsIt(t *testing.T) {
	orgPatch, unspellable, err := filterImage("organization",
		json.RawMessage(`{"display_name":"Weber GmbH","name_source":"domain"}`))
	if err != nil {
		t.Fatalf("filter the organization image: %v", err)
	}
	if _, kept := orgPatch["name_source"]; kept {
		t.Error("a rename's provenance stamp travelled back into the record")
	}
	if _, kept := orgPatch["display_name"]; !kept {
		t.Error("the name the entry actually changed did not survive the filter")
	}
	// Dropped, not refused: an unspellable key would make the whole entry
	// un-undoable, which is the limit this filtering removes.
	if len(unspellable) != 0 {
		t.Errorf("unspellable = %v, want the stamp dropped silently", unspellable)
	}

	leadPatch, _, err := filterImage("lead",
		json.RawMessage(`{"full_name":"Anna Weber","source":"webinar"}`))
	if err != nil {
		t.Fatalf("filter the lead image: %v", err)
	}
	if _, kept := leadPatch["source"]; !kept {
		t.Error("a lead's own source was dropped, so undoing an edit to it would " +
			"leave the changed value in place and report success")
	}
}
