// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Who may take back a stop, against a real database.
//
// The rule is one line — you may lift a decision made below your level — and
// every arm of it decides whether mail reaches somebody who asked us not to be
// written to. So the matrix runs against real rows rather than a fake store: a
// level is a column, the comparison happens inside the transaction that
// revokes, and neither is observable from a double.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// plantSuppression writes a row at a named level, bypassing the write door so a
// lift test can start from a level that door would never produce — `subject`
// above all, which is the arm that must never be liftable.
func plantSuppression(t *testing.T, e *channelConsentEnv, person ids.PersonID, level string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression
		    (id, person_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, $2, 'subject_request', 'test', 'human:x', $3)`,
		id, person, level); err != nil {
		t.Fatalf("planting a %s-level suppression: %v", level, err)
	}
	return id
}

// stillLive reports whether the row is unrevoked.
func stillLive(t *testing.T, e *channelConsentEnv, id ids.UUID) bool {
	t.Helper()
	var live bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_at IS NULL FROM communication_suppression WHERE id = $1`, id).Scan(&live); err != nil {
		t.Fatalf("reading the suppression back: %v", err)
	}
	return live
}

// TestAnAdminLiftsARepsStop is the case the whole level model exists to allow.
//
// Without it a rep's stop is permanent whatever the contract says, which makes
// the level a claim rather than a rule.
func TestAnAdminLiftsARepsStop(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))

	// setupChannelConsent binds an admin role, so e.ctx is the admin seat.
	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: row, Reason: "they asked us to resume",
	}); err != nil {
		t.Fatalf("an admin lifting a rep's stop: %v", err)
	}
	if stillLive(t, e, row) {
		t.Error("the stop is still live after an admin lifted it")
	}
}

// TestARepDoesNotLiftAnotherRepsStop holds the equal-rank arm.
//
// Two reps disagreeing about a contact is a real situation, and the answer is
// not that whoever acts second wins. Reversing a peer's judgement stays a
// deliberate escalation rather than something a rep does by pressing a button.
func TestARepDoesNotLiftAnotherRepsStop(t *testing.T) {
	e := setupChannelConsent(t)
	own := ids.New[ids.PersonKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Their Own Contact', 'test', 'human:x', 'workspace', $2)`,
		own, e.user); err != nil {
		t.Fatal(err)
	}
	row := plantSuppression(t, e, own, string(commsauthz.LevelUser))

	err := e.store.Lift(boundedRepCtx(e.ws, e.user), LiftInput{
		PersonID: own, SuppressionID: row, Reason: "I think they changed their mind",
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep lifting a peer's stop answered %v, want ErrPermissionDenied", err)
	}
	if !stillLive(t, e, row) {
		t.Error("a refused lift revoked the row anyway")
	}
}

// TestNobodyLiftsTheSubjectsOwnAct is the arm that carries a legal obligation
// rather than a product preference.
//
// An Art. 21 objection is absolute. A product whose admin could lift one would
// be offering a control that cannot lawfully be used, and whoever pressed it
// would believe the send was permitted.
func TestNobodyLiftsTheSubjectsOwnAct(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantSuppression(t, e, e.person, string(commsauthz.LevelSubject))

	// The admin seat, which outranks every other level there is.
	err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: row, Reason: "we would like to resume",
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an admin lifting the subject's own act answered %v, want ErrPermissionDenied", err)
	}
	if !stillLive(t, e, row) {
		t.Error("the subject's own stop was revoked; nothing in the installation may do that")
	}
}

// TestAnAlreadyLiftedStopAnswersNotFound keeps the second attempt honest.
//
// It answers alike with a row that belongs to another subject and one that
// never existed, so a caller learns nothing about rows they were not going to
// be allowed to touch.
func TestAnAlreadyLiftedStopAnswersNotFound(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))

	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: row, Reason: "first",
	}); err != nil {
		t.Fatalf("the first lift: %v", err)
	}
	err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: row, Reason: "second",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("lifting an already-lifted stop answered %v, want ErrNotFound", err)
	}

	// And an id belonging to nobody answers the same way.
	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: ids.NewV7(), Reason: "x",
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("lifting an unknown id answered %v, want ErrNotFound", err)
	}
}

// TestALiftedStopStopsRefusingTheSend is the point of the whole verb.
//
// The engine reads the LIVE rows, so a revoked one must stop binding. A lift
// that recorded its own audit entry and left the engine still refusing would
// look like it worked and change nothing a rep can see.
func TestALiftedStopStopsRefusingTheSend(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))

	var liveBefore int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE person_id = $1 AND revoked_at IS NULL`, e.person).Scan(&liveBefore); err != nil {
		t.Fatal(err)
	}
	if liveBefore != 1 {
		t.Fatalf("the fixture planted %d live stops, want 1", liveBefore)
	}

	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: row, Reason: "resolved on a call",
	}); err != nil {
		t.Fatalf("lifting: %v", err)
	}

	var liveAfter int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE person_id = $1 AND revoked_at IS NULL`, e.person).Scan(&liveAfter); err != nil {
		t.Fatal(err)
	}
	if liveAfter != 0 {
		t.Errorf("%d stop(s) still live after the lift; the engine reads live rows and would "+
			"keep refusing", liveAfter)
	}
}

// TestTheLiftCarriesItsAuditAndItsEvent holds the write shape on the way back.
//
// Taking back a stop is the change most worth being able to explain later: the
// row said somebody asked us not to write, and this says who overruled that.
func TestTheLiftCarriesItsAuditAndItsEvent(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))

	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: row, Reason: "resolved on a call",
	}); err != nil {
		t.Fatalf("lifting: %v", err)
	}

	var payload string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT after::text FROM audit_log
		 WHERE entity_type = 'person' AND entity_id = $1 AND action = 'update'
		   AND after ? 'lifted_suppression'`, e.person).Scan(&payload); err != nil {
		t.Fatalf("the lift left no audit entry naming the row it took back: %v", err)
	}
	// Both levels, because an auditor checking that the lift was permitted needs
	// to see the comparison rather than re-derive it from a row that no longer
	// says what it was.
	for _, want := range []string{"recorded_at_level", "lifted_by_level"} {
		if !strings.Contains(payload, want) {
			t.Errorf("the audit payload omits %q: %s", want, payload)
		}
	}
	// The reason the lifter gave. A required field the code discards is worse
	// than an absent one: the contract promises the change is explainable and
	// nothing would hold the words.
	if !strings.Contains(payload, "resolved on a call") {
		t.Errorf("the audit payload omits the reason the lift was made for: %s", payload)
	}

	var events int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'consent.suppression_lifted'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("staged %d lift events, want 1 — a consumer that saw only the suppression "+
			"would treat the subject as stopped forever", events)
	}
}

// TestALiftIsRefusedOnAContactOutsideTheCallersScope holds the VISIBILITY half.
//
// An unpromoted capture is invisible to every seat but its owner's, and
// EnsureVisible runs first, so this answers not-found: a caller learns nothing
// about rows they were never going to reach.
func TestALiftIsRefusedOnAContactOutsideTheCallersScope(t *testing.T) {
	e := setupChannelConsent(t)

	hidden := seedForeignPerson(t, e, "owner")
	// A MACHINE-level stop, which every seat outranks — so a refusal here is the
	// probe refusing and not the level rule.
	row := plantSuppression(t, e, hidden, string(commsauthz.LevelMachine))

	err := e.store.Lift(boundedRepCtx(e.ws, e.user), LiftInput{
		PersonID: hidden, SuppressionID: row, Reason: "looks stale to me",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("lifting on an invisible contact answered %v, want ErrNotFound", err)
	}
	if !stillLive(t, e, row) {
		t.Error("a refused lift revoked the row anyway")
	}
}

// TestALiftIsRefusedOnAContactTheCallerMayOnlyRead holds the WRITE-AUTHORITY
// half, which fails independently and which the visibility arm cannot reach.
//
// The contact is workspace-visible, so this rep CAN see it — EnsureVisible
// passes and only ensureWriteAuthority refuses. That is the half EnsureWritable
// adds over EnsureVisible, and without this arm swapping one for the other goes
// unnoticed.
func TestALiftIsRefusedOnAContactTheCallerMayOnlyRead(t *testing.T) {
	e := setupChannelConsent(t)

	theirs := seedForeignPerson(t, e, "workspace")
	row := plantSuppression(t, e, theirs, string(commsauthz.LevelMachine))

	err := e.store.Lift(boundedRepCtx(e.ws, e.user), LiftInput{
		PersonID: theirs, SuppressionID: row, Reason: "looks stale to me",
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("lifting on a read-only contact answered %v, want ErrPermissionDenied", err)
	}
	if !stillLive(t, e, row) {
		t.Error("a refused lift revoked the row anyway")
	}
}

// seedForeignPerson plants a person owned by somebody other than the test's rep,
// at the given visibility.
func seedForeignPerson(t *testing.T, e *channelConsentEnv, visibility string) ids.PersonID {
	t.Helper()
	person := ids.New[ids.PersonKind]()
	owner := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Owning rep')`,
		owner, "owner-"+owner.String()+"@cc.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Somebody Else''s Contact', 'test', 'human:x', $2, $3)`,
		person, visibility, owner); err != nil {
		t.Fatal(err)
	}
	return person
}

// TestALiftNeedsAReasonSomebodyCanReview holds the field the contract requires.
//
// Both arms, because they fail for different reasons and a single test would
// leave whichever it skipped uncovered. The bound in particular is a number
// written twice — here and in the contract's maxLength — and nothing generated
// enforces the contract's copy.
func TestALiftNeedsAReasonSomebodyCanReview(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))

	for name, reason := range map[string]string{
		"empty":      "",
		"whitespace": "   \t\n ",
		"too long":   strings.Repeat("x", reasonMax+1),
	} {
		err := e.store.Lift(e.ctx, LiftInput{
			PersonID: e.person, SuppressionID: row, Reason: reason,
		})
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("a %s reason was refused with %v, want a validation error", name, err)
			continue
		}
		if invalid.Field != "reason" {
			t.Errorf("a %s reason was refused on field %q, want reason", name, invalid.Field)
		}
	}

	// And none of them revoked anything on the way past.
	if !stillLive(t, e, row) {
		t.Error("a refused lift revoked the row anyway")
	}

	// The bound admits its own limit, so the check is a ceiling and not an
	// off-by-one that refuses a legitimate 500-character explanation.
	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID: e.person, SuppressionID: row, Reason: strings.Repeat("x", reasonMax),
	}); err != nil {
		t.Errorf("a reason at exactly the limit was refused: %v", err)
	}
}

// lastLiftPayload decodes the consent.suppression_lifted envelope the lift just
// staged. Read from the outbox rather than returned by the store, because the
// row a consumer will actually receive is the thing under test.
func lastLiftPayload(t *testing.T, e *channelConsentEnv) crmcontracts.PublicEventConsentSuppressionLifted {
	t.Helper()
	var raw []byte
	if err := e.owner.QueryRow(context.Background(), `
		SELECT envelope->'payload' FROM event_outbox
		 WHERE envelope->>'type' = 'consent.suppression_lifted'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`).Scan(&raw); err != nil {
		t.Fatalf("reading the staged lift event: %v", err)
	}
	var payload crmcontracts.PublicEventConsentSuppressionLifted
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decoding the lift payload: %v", err)
	}
	return payload
}

// TestALiftSaysWhatStillStands is the whole reason the event carries more than
// two authority levels.
//
// A person can hold several stops at once — their own objection and a rep's
// separate note. Lifting one leaves the others standing, and an event that says
// only "a stop was lifted" reads to an outside consumer as "you may write to
// them now". Margince itself is safe either way, because the engine re-reads the
// strongest live row inside the sending transaction; the consumers reading this
// event are the ones that would resume mail to somebody who objected.
func TestALiftSaysWhatStillStands(t *testing.T) {
	e := setupChannelConsent(t)

	// Two stops. The user-level one is liftable by an admin; the subject's own
	// is not, and is what must still be reported after the lift.
	liftable := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))
	subjects := plantSuppression(t, e, e.person, string(commsauthz.LevelSubject))

	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID:      e.person,
		SuppressionID: liftable,
		Reason:        "the rep confirmed this note was filed against the wrong contact",
	}); err != nil {
		t.Fatalf("lifting the user-level stop as admin: %v", err)
	}

	payload := lastLiftPayload(t, e)
	if payload.SuppressionId == nil || *payload.SuppressionId != openapi_types.UUID(liftable) {
		t.Errorf("event names suppression %s, want the one that was lifted (%s)",
			derefUUID(payload.SuppressionId), liftable)
	}
	if payload.RemainingSuppressions == nil || *payload.RemainingSuppressions != 1 {
		t.Errorf("remaining_suppressions = %d, want 1: the subject's own objection still stands",
			payload.RemainingSuppressions)
	}
	if payload.StillSuppressed == nil || !*payload.StillSuppressed {
		t.Error("still_suppressed = false while the subject's objection is live — " +
			"a consumer reading this event would resume mail to somebody who objected")
	}
	if !stillLive(t, e, subjects) {
		t.Error("the subject's objection was revoked by a lift aimed at another row")
	}
}

// TestALiftOfTheLastStopSaysSo is the other direction: when nothing remains,
// the event must say so, or a consumer holding mail back forever is the bug.
func TestALiftOfTheLastStopSaysSo(t *testing.T) {
	e := setupChannelConsent(t)
	only := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))

	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID:      e.person,
		SuppressionID: only,
		Reason:        "recorded in error against this contact",
	}); err != nil {
		t.Fatalf("lifting the only stop: %v", err)
	}

	payload := lastLiftPayload(t, e)
	if payload.RemainingSuppressions == nil || *payload.RemainingSuppressions != 0 ||
		payload.StillSuppressed == nil || *payload.StillSuppressed {
		t.Errorf("remaining = %d, still_suppressed = %v; want 0 and false with no stop left",
			derefInt(payload.RemainingSuppressions), derefBool(payload.StillSuppressed))
	}
}

// TestALiftReportsAnAddressPinnedStopAsStanding holds the count's match rule.
//
// A hard bounce is pinned to an ADDRESS and carries no person_id. The engine
// still refuses every message to that mailbox, because liveSuppression matches
// person OR lead OR address. A count that asked only about person_id would
// answer "nothing stands" for exactly that subject, and still_suppressed would
// tell a consumer to resume mail the engine will not send. Under-reporting is
// the one direction this field must never fail in.
func TestALiftReportsAnAddressPinnedStopAsStanding(t *testing.T) {
	e := setupChannelConsent(t)

	address := "bounced-" + e.person.String() + "@example.test"
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
		 VALUES ($1, lower($2), true, 'test', 'human:x')`, e.person, address); err != nil {
		t.Fatalf("giving the person an address: %v", err)
	}
	// Pinned to the address alone, the way a bounce handler writes it.
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO communication_suppression
		     (id, address, kind, source, captured_by, decided_by_level)
		 VALUES ($1, lower($2), 'hard_bounce', 'test', 'system', 'machine')`,
		ids.NewV7(), address); err != nil {
		t.Fatalf("planting the address-pinned bounce: %v", err)
	}

	liftable := plantSuppression(t, e, e.person, string(commsauthz.LevelUser))
	if err := e.store.Lift(e.ctx, LiftInput{
		PersonID:      e.person,
		SuppressionID: liftable,
		Reason:        "filed against the wrong contact",
	}); err != nil {
		t.Fatalf("lifting the person-level stop: %v", err)
	}

	payload := lastLiftPayload(t, e)
	if payload.StillSuppressed == nil || !*payload.StillSuppressed ||
		payload.RemainingSuppressions == nil || *payload.RemainingSuppressions != 1 {
		t.Errorf("remaining = %d, still_suppressed = %v; want 1 and true — the bounce on "+
			"%s is still live and the engine will still refuse this mail",
			derefInt(payload.RemainingSuppressions), derefBool(payload.StillSuppressed), address)
	}
}

// The three fields are optional on the wire so an older consumer keeps
// validating, but this writer always sets them — so a nil in a failure message
// is itself the news, and these print it rather than an address.
func derefInt(v *int) any {
	if v == nil {
		return "absent"
	}
	return *v
}

func derefBool(v *bool) any {
	if v == nil {
		return "absent"
	}
	return *v
}

func derefUUID(v *openapi_types.UUID) any {
	if v == nil {
		return "absent"
	}
	return *v
}
