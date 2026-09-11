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
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// liveSuppressionRow reads back what the write actually stored.
func liveSuppressionRow(t *testing.T, e *channelConsentEnv, contact ids.ContactID) (kind, level, source string) {
	t.Helper()
	err := e.owner.QueryRow(context.Background(), `
		SELECT kind, decided_by_level, source
		  FROM communication_suppression
		 WHERE contact_id = $1 AND revoked_at IS NULL`, contact).Scan(&kind, &level, &source)
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
		ContactID: e.contact,
		Kind:      "subject_request",
		Reason:    "asked on the phone to stop",
	})
	if err != nil {
		t.Fatalf("recording the suppression: %v", err)
	}

	kind, level, source := liveSuppressionRow(t, e, e.contact)
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
	if source == "" || source == "recorded by a contact" {
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
		ContactID: e.contact, Kind: "subject_request",
	}); err != nil {
		t.Fatalf("recording the suppression: %v", err)
	}

	var audits int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'update'`,
		e.contact).Scan(&audits); err != nil {
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

// TestOnlyAStopTheSubjectAskedForIsRecordableByHand bounds the door.
//
// A processing restriction is an Art. 18 legal state with its own workflow, and
// a hard bounce is a fact only the mail path observes. A door accepting either
// would let a rep write, in good faith, a row asserting something nobody
// verified.
//
// marketing_objection USED TO BE REFUSED HERE too, on that reasoning plus the
// observation that it is unliftable, so a mistake would be permanent. That read
// the article backwards: Art. 21(2) is an unconditional right the subject
// exercises "at any time" and by any means, so demanding a form or a
// self-service link before one can be written puts a condition on it. The
// practical cost exceeded the theoretical one — nothing wrote the kind at all,
// so a rep told "stop the newsletter" reached for subject_request, which
// stopped that contact's invoices too.
//
// The permanence is real and accepted: an objection is undone by the subject
// reversing it or by the per-message exception path, never by a seat. A stop
// too hard to lift costs an awkward conversation; one too easy costs mail
// somebody explicitly refused.
func TestOnlyAStopTheSubjectAskedForIsRecordableByHand(t *testing.T) {
	e := setupChannelConsent(t)

	for _, kind := range []string{"processing_restriction", "hard_bounce", ""} {
		err := e.store.Suppress(e.ctx, SuppressInput{ContactID: e.contact, Kind: kind})
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
		`SELECT count(*) FROM communication_suppression WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
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
		ContactID: ids.New[ids.ContactKind](), Kind: "subject_request",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("suppressing an unknown subject answered %v, want ErrNotFound so existence stays hidden", err)
	}
}

// TestAContactAnotherRepCannotSeeIsNotSuppressible is the arm that matters, and
// the one a nonexistent id cannot reach.
//
// `contact` carries capture privacy: a mailbox sync auto-creates rows as
// `owner`, visible to the capturing user alone until a human promotes them. A
// bare existence check would let any seat with contact.update write a permanent
// stop onto a contact they cannot open — a refusal its owner sees on every send
// with nothing on screen explaining it — and the 204/404 difference would tell
// the caller which ids are real.
func TestAContactAnotherRepCannotSeeIsNotSuppressible(t *testing.T) {
	e := setupChannelConsent(t)

	// A contact the mailbox sync invented for somebody else, never promoted.
	other := ids.New[ids.ContactKind]()
	owner := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other rep')`,
		owner, "other-"+owner.String()+"@cc.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Unpromoted Contact', 'test', 'human:x', 'owner', $2)`,
		other, owner); err != nil {
		t.Fatal(err)
	}

	err := e.store.Suppress(boundedRepCtx(e.ws, e.user), SuppressInput{
		ContactID: other, Kind: "subject_request",
	})
	if err == nil {
		t.Fatal("a rep stopped a contact they cannot see; the write must run the row-scope probe")
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression WHERE contact_id = $1`, other).Scan(&rows); err != nil {
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
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		},
	})

	// The sentinel, not just any error: 403 and 404 mean different things here,
	// and a reader who may SEE the contact should learn they may not write —
	// not be told the contact does not exist.
	err := e.store.Suppress(readOnly, SuppressInput{ContactID: e.contact, Kind: "subject_request"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader was refused with %v, want ErrPermissionDenied", err)
	}
}

// TestARepsRowIsWrittenAtTheRepsOwnLevel is the arm the admin harness cannot
// reach, and the one the whole authority model rests on.
//
// A rep's row must be `user`: liftable by an admin, and not by another rep. A
// row written at `subject` would be liftable by nobody in the installation —
// a permanent stop on one contact, written by any seat holding contact.update,
// which is the denial of service the level system exists to prevent.
func TestARepsRowIsWrittenAtTheRepsOwnLevel(t *testing.T) {
	e := setupChannelConsent(t)

	// A contact this rep owns. EnsureWritable's write-authority arm refuses a
	// bounded seat on somebody else's record, so a rep arm that used the shared
	// fixture would be testing the refusal rather than the level.
	own := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Their Own Contact', 'test', 'human:x', 'workspace', $2)`,
		own, e.user); err != nil {
		t.Fatal(err)
	}

	if err := e.store.Suppress(boundedRepCtx(e.ws, e.user), SuppressInput{
		ContactID: own, Kind: "subject_request",
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
	theirs := ids.New[ids.ContactKind]()
	owner := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Owning rep')`,
		owner, "owner-"+owner.String()+"@cc.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Somebody Else''s Contact', 'test', 'human:x', 'workspace', $2)`,
		theirs, owner); err != nil {
		t.Fatal(err)
	}

	err := e.store.Suppress(boundedRepCtx(e.ws, e.user), SuppressInput{
		ContactID: theirs, Kind: "subject_request",
	})
	if err == nil {
		t.Fatal("a rep stopped a contact they may only read; the probe must check write authority, not visibility")
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression WHERE contact_id = $1`, theirs).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the refused write still left %d row(s) on a contact the rep may only read", rows)
	}
}

// TestASuppressionNeedsAReasonSomebodyCanReview holds the same bound on this
// door that TestALiftNeedsAReasonSomebodyCanReview holds on its sibling.
//
// The two doors take the same field under the same contract limit and enforced
// it in one place only: recording a stop accepted a megabyte where taking one
// back refused at 500. Both now call requireReason, and this test is what fails
// if one of them stops.
func TestASuppressionNeedsAReasonSomebodyCanReview(t *testing.T) {
	e := setupChannelConsent(t)

	// Only the BOUND. Empty is legal here and refused on the lift door, because
	// the contract says so: the suppress body is `required: [kind]`, and a rep
	// relaying "please stop emailing me" may have nothing to add. Asserting a
	// refusal on empty would pin behaviour that breaks a conforming client.
	for name, reason := range map[string]string{
		"too long": strings.Repeat("x", reasonMax+1),
	} {
		err := e.store.Suppress(e.ctx, SuppressInput{
			ContactID: e.contact, Kind: suppressibleKind, Reason: reason,
		})
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("a %s reason was refused with %v, want a validation error", name, err)
			continue
		}
		if invalid.Field != fieldReason {
			t.Errorf("a %s reason named field %q, want %q", name, invalid.Field, fieldReason)
		}
	}
}

// TestASuppressionMayCarryNoReason pins the other half of the contract: this
// door's reason is OPTIONAL, and a body of `{"kind":"subject_request"}` is one
// the published contract accepts. A shared validator that demanded a reason
// here would answer 422 to that body and stop a rep recording a stop somebody
// actually asked for.
func TestASuppressionMayCarryNoReason(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: suppressibleKind,
	}); err != nil {
		t.Fatalf("recording a stop with no reason: %v", err)
	}
	if kind, _, _ := liveSuppressionRow(t, e, e.contact); kind != suppressibleKind {
		t.Errorf("kind = %q, want %q", kind, suppressibleKind)
	}
}

// TestARepRecordsAnObjectionAtTheSubjectsOwnLevel is the capability R4 named as
// missing: nothing in production wrote marketing_objection, so the only stop a
// rep could record was the broad one.
//
// The LEVEL is the whole point of the test. Art. 21 gives the right to the data
// subject, so the row records the subject's authority and not the rep's, and
// CanOverrule refuses to rank anything above LevelSubject. A row written at the
// rep's own level would be liftable by any admin — an installation quietly
// undoing a stop the contact asked for.
func TestARepRecordsAnObjectionAtTheSubjectsOwnLevel(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact,
		Kind:      commsauthz.ReasonObjection,
		Reason:    "asked on the phone to stop the newsletter",
	}); err != nil {
		t.Fatalf("recording an objection: %v", err)
	}

	kind, level, _ := liveSuppressionRow(t, e, e.contact)
	if kind != commsauthz.ReasonObjection {
		t.Errorf("kind = %q, want %q", kind, commsauthz.ReasonObjection)
	}
	if level != string(commsauthz.LevelSubject) {
		t.Errorf("decided_by_level = %q, want %q — an objection recorded at the seat's own level "+
			"is one the next admin can lift", level, commsauthz.LevelSubject)
	}
}

// TestNoSeatLiftsAnObjectionItRecorded is the consequence of that level, proved
// through the real lift door rather than by reasoning about CanOverrule.
//
// The admin here holds every grant the lift door asks for. What refuses them is
// the authority ON THE ROW, which is the subject's.
func TestNoSeatLiftsAnObjectionItRecorded(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "stop the newsletter",
	}); err != nil {
		t.Fatalf("recording an objection: %v", err)
	}

	// THE ROW MUST BE NAMED, or this proves nothing.
	//
	// admitLift refuses a zero SuppressionID as a validation error before it
	// ever compares authority, so a LiftInput without one passes this test with
	// the subject-level stamp deleted — asserting the shape of the request
	// rather than who may lift. Found by mutation, which is the only reason
	// this line exists.
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT id FROM communication_suppression
		 WHERE contact_id = $1 AND revoked_at IS NULL`, e.contact).Scan(&id); err != nil {
		t.Fatalf("reading back the objection: %v", err)
	}

	// e.ctx binds an ADMIN seat — TestARepRecordsTheSubjectsOwnRequest asserts
	// its rows land at admin level — so this is the strongest seat the product
	// has, holding every grant the lift door asks for. What refuses it is the
	// authority ON THE ROW, which belongs to the subject.
	err := e.store.Lift(e.ctx, LiftInput{
		ContactID: e.contact, SuppressionID: id,
		Reason: "the rep says it was a misunderstanding",
	})
	if err == nil {
		t.Fatal("an admin lifted the subject's own Art. 21 objection")
	}
	// And refused for the RIGHT reason: a validation error here would mean the
	// request was malformed, not that the authority held.
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		t.Fatalf("the lift was refused as malformed (%v), so this says nothing about authority", err)
	}
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the lift was refused with %v, want a permission denial naming the level", err)
	}

	// WHAT MAKES THIS SUBJECT-LEVEL AND NOT MERELY ADMIN-LEVEL. CanOverrule is
	// strictly greater, so an admin cannot lift an admin row either and the
	// refusal above reads the same for both. The difference is who COULD: an
	// admin-level row is liftable by anything ranking above admin, and a
	// subject-level row is liftable by nothing at all, which is the whole
	// reason Art. 21 rows are stamped that way.
	//
	// So the level itself is the assertion. Without it this test passes with
	// the subject-level stamp deleted — found by mutation.
	if _, level, _ := liveSuppressionRow(t, e, e.contact); level != string(commsauthz.LevelSubject) {
		t.Errorf("the surviving row is at %q, not %q: an admin-level objection is one a higher "+
			"rank could still lift", level, commsauthz.LevelSubject)
	}

	if kind, _, _ := liveSuppressionRow(t, e, e.contact); kind != commsauthz.ReasonObjection {
		t.Errorf("the objection is no longer the live row (kind = %q) after a refused lift", kind)
	}
}

// TestAnObjectionLeavesTheInvoiceAlone is R3 from the other side, and the
// reason the objection kind had to exist at all.
//
// Before this a rep told "stop the newsletter" had only subject_request, which
// bound every category — so the contact's invoices stopped with their marketing.
// An objection reaches marketing and nothing else.
func TestAnObjectionLeavesTheInvoiceAlone(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "no more newsletters",
	}); err != nil {
		t.Fatalf("recording an objection: %v", err)
	}

	for _, c := range []struct {
		category commsauthz.Category
		stopped  bool
	}{
		{commsauthz.CategoryMarketing, true},
		{commsauthz.CategoryInvoiceOrPayment, false},
		{commsauthz.CategoryReplyToInbound, false},
		{commsauthz.CategoryContractNotice, false},
	} {
		if got := suppressionBinds(commsauthz.ReasonObjection, c.category); got != c.stopped {
			t.Errorf("an objection against %s: binds = %v, want %v", c.category, got, c.stopped)
		}
	}
}

// TestAStopEverythingStillConfirmsItself is the R3 half about subject_request.
//
// "Stop contacting me" reaches nearly everything — and NOT the confirmation
// that we stopped, the privacy notice answering their rights request, or the
// security warning about their own account. Those three are obligations the
// controller owes whatever the subject wants sent, and binding them meant a
// contact who asked us to stop never heard that we had.
func TestAStopEverythingStillConfirmsItself(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: suppressibleKind, Reason: "stop contacting me",
	}); err != nil {
		t.Fatalf("recording the request: %v", err)
	}

	for _, c := range []struct {
		category commsauthz.Category
		stopped  bool
	}{
		{commsauthz.CategoryMarketing, true},
		{commsauthz.CategoryInvoiceOrPayment, true},
		{commsauthz.CategoryReplyToInbound, true},
		{commsauthz.CategoryOptoutConfirmation, false},
		{commsauthz.CategoryPrivacyNotice, false},
		{commsauthz.CategorySecurityNotice, false},
	} {
		if got := suppressionBinds(commsauthz.ReasonSubjectRequest, c.category); got != c.stopped {
			t.Errorf("a subject request against %s: binds = %v, want %v", c.category, got, c.stopped)
		}
	}
}
