// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Recording that we may not write to somebody, against a real database.
//
// The integration lane rather than a unit test because the thing under test is
// what the row LOOKS LIKE to the engine that reads it. liveSuppression takes
// the strongest live row for a subject and applies it to every category, so a
// write that landed with the wrong kind, the wrong level or no row at all is a
// defect no fake store could show — and the engine reads this table on every
// send, so it is live the moment this lands.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// liveSuppressionRow reads back what the write actually stored.
func liveSuppressionRow(t *testing.T, e *channelConsentEnv, person ids.PersonID) (kind, level, source string) {
	t.Helper()
	err := e.owner.QueryRow(context.Background(), `
		SELECT kind, decided_by_level, source
		  FROM communication_suppression
		 WHERE person_id = $1 AND revoked_at IS NULL`, person).Scan(&kind, &level, &source)
	if err != nil {
		t.Fatalf("reading back the suppression: %v", err)
	}
	return kind, level, source
}

// TestARepRecordsTheSubjectsOwnRequest is the capability that did not exist.
//
// Before this the table was read by the engine, deleted by erasure and exported
// by SAR, and written by nothing — so a rep told on a call to stop had nowhere
// to put it.
func TestARepRecordsTheSubjectsOwnRequest(t *testing.T) {
	e := setupChannelConsent(t)

	err := e.store.Suppress(e.ctx, SuppressInput{
		PersonID: e.person,
		Kind:     "subject_request",
		Reason:   "asked on the phone to stop",
	})
	if err != nil {
		t.Fatalf("recording the suppression: %v", err)
	}

	kind, level, source := liveSuppressionRow(t, e, e.person)
	if kind != "subject_request" {
		t.Errorf("kind = %q, want subject_request", kind)
	}
	// EXACTLY the seat's own level. The harness binds an admin role, so an
	// either-or assertion would pass with authorityOf returning a constant —
	// and the rep arm, which is the one that must not be `subject`, would never
	// run. TestARepsRowIsWrittenAtTheRepsOwnLevel covers the other seat.
	if level != string(commsauthz.LevelAdmin) {
		t.Errorf("decided_by_level = %q, want admin for an admin seat", level)
	}
	// What the rep was told survives, because a suppression somebody later asks
	// to lift is only reviewable if the record says why it was made.
	if source == "" || source == "recorded by a person" {
		t.Errorf("source = %q, want it to carry the reason the rep typed", source)
	}
}

// TestTheWriteCarriesItsAuditAndItsEvent holds the write shape.
//
// A domain row without them is a change nobody can trace: the audit entry is
// what answers "who stopped this contact and when", and the outbox row is what
// lets anything downstream react. They commit together or not at all.
func TestTheWriteCarriesItsAuditAndItsEvent(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		PersonID: e.person, Kind: "subject_request",
	}); err != nil {
		t.Fatalf("recording the suppression: %v", err)
	}

	var audits int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'person' AND entity_id = $1 AND action = 'update'`,
		e.person).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Errorf("wrote %d audit rows, want exactly 1 — a suppression nobody can trace "+
			"cannot answer who stopped this contact", audits)
	}

	var events int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'consent.suppressed'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("staged %d outbox rows, want exactly 1", events)
	}
}

// TestOnlyTheSubjectsOwnRequestIsRecordableByHand bounds the door.
//
// An objection and a processing restriction carry legal consequences a relayed
// phone call does not establish, and a hard bounce is a fact only the mail path
// observes. A door that accepted them would let a rep write, in good faith, a
// row asserting something nobody verified — and marketing_objection is
// unliftable, so the mistake would be permanent.
func TestOnlyTheSubjectsOwnRequestIsRecordableByHand(t *testing.T) {
	e := setupChannelConsent(t)

	for _, kind := range []string{"marketing_objection", "processing_restriction", "hard_bounce", ""} {
		err := e.store.Suppress(e.ctx, SuppressInput{PersonID: e.person, Kind: kind})
		// A VALIDATION error naming the field, not merely some error. Without
		// this the test stays green with the check deleted — a bad kind would
		// reach the table's CHECK constraint and fail there, handing the caller
		// a 500 that leaks a constraint name instead of a 422 saying what to fix.
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("kind %q was refused with %v, want a validation error", kind, err)
			continue
		}
		if invalid.Field != "kind" {
			t.Errorf("kind %q was refused on field %q, want kind", kind, invalid.Field)
		}
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression WHERE person_id = $1`, e.person).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("a refused kind still wrote %d row(s)", rows)
	}
}

// TestAnUnknownSubjectIsNotSuppressible closes the plainest arm: an id that
// names nothing answers not-found rather than writing a row.
func TestAnUnknownSubjectIsNotSuppressible(t *testing.T) {
	e := setupChannelConsent(t)

	err := e.store.Suppress(e.ctx, SuppressInput{
		PersonID: ids.New[ids.PersonKind](), Kind: "subject_request",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("suppressing an unknown subject answered %v, want ErrNotFound so existence stays hidden", err)
	}
}

// TestAContactAnotherRepCannotSeeIsNotSuppressible is the arm that matters, and
// the one a nonexistent id cannot reach.
//
// `person` carries capture privacy: a mailbox sync auto-creates rows as
// `owner`, visible to the capturing user alone until a human promotes them. A
// bare existence check would let any seat with person.update write a permanent
// stop onto a contact they cannot open — a refusal its owner sees on every send
// with nothing on screen explaining it — and the 204/404 difference would tell
// the caller which ids are real.
func TestAContactAnotherRepCannotSeeIsNotSuppressible(t *testing.T) {
	e := setupChannelConsent(t)

	// A contact the mailbox sync invented for somebody else, never promoted.
	other := ids.New[ids.PersonKind]()
	owner := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other rep')`,
		owner, "other-"+owner.String()+"@cc.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Unpromoted Contact', 'test', 'human:x', 'owner', $2)`,
		other, owner); err != nil {
		t.Fatal(err)
	}

	err := e.store.Suppress(boundedRepCtx(e.ws, e.user), SuppressInput{
		PersonID: other, Kind: "subject_request",
	})
	if err == nil {
		t.Fatal("a rep stopped a contact they cannot see; the write must run the row-scope probe")
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression WHERE person_id = $1`, other).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the refused write still left %d suppression row(s) on somebody else's contact", rows)
	}
}

// TestASeatWithoutWriteAuthorityIsRefused proves the gate is the store's, not
// the handler's.
func TestASeatWithoutWriteAuthorityIsRefused(t *testing.T) {
	e := setupChannelConsent(t)

	readOnly := principal.WithWorkspaceID(context.Background(), e.ws)
	readOnly = principal.WithCorrelationID(readOnly, ids.NewV7())
	readOnly = principal.WithActor(readOnly, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"person": {Read: true}},
		},
	})

	// The sentinel, not just any error: 403 and 404 mean different things here,
	// and a reader who may SEE the person should learn they may not write —
	// not be told the person does not exist.
	err := e.store.Suppress(readOnly, SuppressInput{PersonID: e.person, Kind: "subject_request"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader was refused with %v, want ErrPermissionDenied", err)
	}
}

// TestARepsRowIsWrittenAtTheRepsOwnLevel is the arm the admin harness cannot
// reach, and the one the whole authority model rests on.
//
// A rep's row must be `user`: liftable by an admin, and not by another rep. A
// row written at `subject` would be liftable by nobody in the installation —
// a permanent stop on one contact, written by any seat holding person.update,
// which is the denial of service the level system exists to prevent.
func TestARepsRowIsWrittenAtTheRepsOwnLevel(t *testing.T) {
	e := setupChannelConsent(t)

	// A contact this rep owns. EnsureWritable's write-authority arm refuses a
	// bounded seat on somebody else's record, so a rep arm that used the shared
	// fixture would be testing the refusal rather than the level.
	own := ids.New[ids.PersonKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Their Own Contact', 'test', 'human:x', 'workspace', $2)`,
		own, e.user); err != nil {
		t.Fatal(err)
	}

	if err := e.store.Suppress(boundedRepCtx(e.ws, e.user), SuppressInput{
		PersonID: own, Kind: "subject_request",
	}); err != nil {
		t.Fatalf("a rep recording a suppression on their own contact: %v", err)
	}

	_, level, _ := liveSuppressionRow(t, e, own)
	if level != string(commsauthz.LevelUser) {
		t.Errorf("decided_by_level = %q for a rep, want user so an admin can lift it", level)
	}
}

// TestARepCannotStopAContactTheyOnlyRead is the write-authority half, and it is
// separate from the visibility half because the two fail independently.
//
// EnsureWritable is EnsureVisible PLUS an owner/team/grant check. A contact
// shared into a rep's view read-only is VISIBLE to them, so a probe that only
// asked about visibility would let that rep write a permanent stop onto
// somebody else's record — a refusal its owner sees on every send with nothing
// on screen explaining it.
func TestARepCannotStopAContactTheyOnlyRead(t *testing.T) {
	e := setupChannelConsent(t)

	// Owned by somebody else, but workspace-visible: every seat can READ it.
	theirs := ids.New[ids.PersonKind]()
	owner := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Owning rep')`,
		owner, "owner-"+owner.String()+"@cc.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Somebody Else''s Contact', 'test', 'human:x', 'workspace', $2)`,
		theirs, owner); err != nil {
		t.Fatal(err)
	}

	err := e.store.Suppress(boundedRepCtx(e.ws, e.user), SuppressInput{
		PersonID: theirs, Kind: "subject_request",
	})
	if err == nil {
		t.Fatal("a rep stopped a contact they may only read; the probe must check write authority, not visibility")
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression WHERE person_id = $1`, theirs).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the refused write still left %d row(s) on a contact the rep may only read", rows)
	}
}
