// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Taking a disclosure duty and ending one without sending anything, against a
// real database — because every rule these writes hold is a CHECK constraint or
// a row lock, and neither exists in a fake.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// officerCtx is the env's context with the privacy-queue grant added.
//
// The shared harness seats a rep holding `contact`, which every other test in
// this package needs and none of these do: a notice case is gated on
// `privacy_request`, the same object the subject-request queue uses, so an
// installation that delegated its privacy inbox delegated this with it.
func officerCtx(e *channelConsentEnv) context.Context {
	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"contact":         {Create: true, Read: true, Update: true, Delete: true},
		"privacy_request": {Create: true, Read: true, Update: true, Delete: true},
	}
	return principal.WithActor(e.ctx, actor)
}

// TestTakingADutyShowsWhoIsWorkingIt is the queue's whole reason for the
// assigned state: an owned case reads differently from an unclaimed one.
func TestTakingADutyShowsWhoIsWorkingIt(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	got, err := e.store.AssignNoticeCase(ctx, id, e.user)
	if err != nil {
		t.Fatalf("taking the duty: %v", err)
	}
	if got.State != NoticeAssigned {
		t.Errorf("an open duty somebody took is %q, want %q", got.State, NoticeAssigned)
	}
	if got.OwnerUserID == nil || *got.OwnerUserID != e.user {
		t.Errorf("the case names owner %v, want the seat that took it (%v)", got.OwnerUserID, e.user)
	}
	if got.AssignedAt == nil {
		t.Error("a taken case says when it was taken; assigned_at is null")
	}
}

// TestAnEndedDutyTakesNoOwner. Assigning a closed case would put it back on the
// queue as work somebody is doing, when the duty is over — the queue's own read
// filters on the unresolved states, so an assigned-and-completed row is a
// contradiction the state column cannot hold.
func TestAnEndedDutyTakesNoOwner(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	if _, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  "Art. 14(5)(b): notice to this contact is impossible, the address bounced permanently",
		Now:   time.Now(),
	}); err != nil {
		t.Fatalf("excusing the duty: %v", err)
	}

	var verr *ValidationError
	_, err := e.store.AssignNoticeCase(ctx, id, e.user)
	if !errors.As(err, &verr) || verr.Field != fieldState {
		t.Fatalf("assigning an ended duty answered %v, want a validation error on %q", err, fieldState)
	}
}

// TestExcusingADutyKeepsTheGround is the difference between these two states
// and not_required, which records the same conclusion with nothing to defend
// it. An auditor asking why a case is closed reads this column.
func TestExcusingADutyKeepsTheGround(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
	when := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	const ground = "Told in person at the trade fair on the 3rd; they took the leaflet"

	got, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeProvidedElsewhere, Note: ground, Now: when,
	})
	if err != nil {
		t.Fatalf("recording that the duty was met elsewhere: %v", err)
	}
	if got.State != NoticeProvidedElsewhere {
		t.Errorf("the case is %q, want %q", got.State, NoticeProvidedElsewhere)
	}
	if got.ResolutionNote == nil || *got.ResolutionNote != ground {
		t.Errorf("the ground reads %v, want the officer's own words", got.ResolutionNote)
	}
	if got.ResolvedBy == nil || *got.ResolvedBy != e.user {
		t.Errorf("the case names resolver %v, want the seat that excused it (%v)", got.ResolvedBy, e.user)
	}
	// The caller's clock, not the database's: it is what lets a test drive a
	// deadline, and a completed_at read from now() would answer a different
	// question from the one this asked.
	if got.CompletedAt == nil || !got.CompletedAt.Equal(when) {
		t.Errorf("the case ended at %v, want the caller's own clock (%v)", got.CompletedAt, when)
	}
}

// TestAnExcuseWithoutAGroundIsRefused, including one made of whitespace — the
// table's CHECK sees a non-null value there and would let " " close a
// compliance record with nothing written on it.
func TestAnExcuseWithoutAGroundIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)

	for _, note := range []string{"", "   \n\t "} {
		id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
		var verr *ValidationError
		_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
			State: NoticeExemptWithReason, Note: note, Now: time.Now(),
		})
		if !errors.As(err, &verr) || verr.Field != fieldResolutionNote {
			t.Errorf("excusing with note %q answered %v, want a validation error on %q",
				note, err, fieldResolutionNote)
		}
	}
}

// TestAGroundLongerThanTheColumnIsRefusedByName. The table holds this too, and
// a raw constraint violation reads as a database fault — the writer gets blamed
// for what is a bad argument.
func TestAGroundLongerThanTheColumnIsRefusedByName(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	var verr *ValidationError
	_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  strings.Repeat("x", noticeNoteLimit+1),
		Now:   time.Now(),
	})
	if !errors.As(err, &verr) || verr.Field != fieldResolutionNote {
		t.Fatalf("an over-long ground answered %v, want a validation error on %q", err, fieldResolutionNote)
	}
}

// TestADischargedDutyIsNotExcusedAfterwards. A later note would overwrite a
// real discharge and then read as the reason it happened — the audit trail
// would say a mail went out because the subject already had the information.
func TestADischargedDutyIsNotExcusedAfterwards(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	if _, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeProvidedElsewhere, Note: "told by a colleague in their own mail", Now: time.Now(),
	}); err != nil {
		t.Fatalf("the first excuse: %v", err)
	}

	var verr *ValidationError
	_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason, Note: "actually it was exempt", Now: time.Now(),
	})
	if !errors.As(err, &verr) || verr.Field != fieldState {
		t.Fatalf("excusing an ended duty answered %v, want a validation error on %q", err, fieldState)
	}
}

// TestAnExcusedDutyLeavesTheQueue is what makes this a discharge rather than a
// note: the case stops being owed, which the queue derives from the terminal
// set rather than a second list.
func TestAnExcusedDutyLeavesTheQueue(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	before, err := e.store.ListNoticeCases(ctx, nil, 0)
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if len(before) != 1 || before[0].ID != id {
		t.Fatalf("the queue holds %d cases before the excuse, want the one seeded", len(before))
	}

	if _, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  "Art. 14(5)(c): the disclosure is laid down by the law we obtained them under",
		Now:   time.Now(),
	}); err != nil {
		t.Fatalf("excusing the duty: %v", err)
	}

	after, err := e.store.ListNoticeCases(ctx, nil, 0)
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("the queue still holds %d cases after the excuse, want none: an excused duty "+
			"is discharged, and a queue that keeps showing it prompts work nobody owes", len(after))
	}

	// It is still readable by name, which is what an auditor asks for.
	closed, err := e.store.ListNoticeCases(ctx, []NoticeState{NoticeExemptWithReason}, 0)
	if err != nil {
		t.Fatalf("reading the excused cases: %v", err)
	}
	if len(closed) != 1 || closed[0].ID != id {
		t.Errorf("asking for excused cases answered %d rows, want the one that was excused", len(closed))
	}
}

// TestTheQueueRefusesAStateThatDoesNotExist. A caller naming a typo would
// otherwise read an empty page and conclude there are no duties, which is the
// one wrong answer a compliance queue must not give quietly.
func TestTheQueueRefusesAStateThatDoesNotExist(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)

	var verr *ValidationError
	_, err := e.store.ListNoticeCases(ctx, []NoticeState{"overdue"}, 0)
	if !errors.As(err, &verr) || verr.Field != fieldState {
		t.Fatalf("asking for a state that does not exist answered %v, want a validation error on %q",
			err, fieldState)
	}
}

// TestTheNoticeQueueIsGatedOnThePrivacyObject. A seat holding `contact` alone
// is the ordinary rep, and these rows say how a named contact was obtained and
// whether anybody has told them yet.
func TestTheNoticeQueueIsGatedOnThePrivacyObject(t *testing.T) {
	e := setupChannelConsent(t)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	if _, err := e.store.ListNoticeCases(e.ctx, nil, 0); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep reading the notice queue got %v, want permission denied", err)
	}
	if _, err := e.store.AssignNoticeCase(e.ctx, id, e.user); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep taking a duty got %v, want permission denied", err)
	}
	if _, err := e.store.ExcuseNoticeCase(e.ctx, id, ExcuseInput{
		State: NoticeExemptWithReason, Note: "because I said so", Now: time.Now(),
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep excusing a duty got %v, want permission denied", err)
	}
}

// TestExcusingRefusesAStateThatIsNotAnExcuse. `completed` means a disclosure
// this installation sent was delivered, and letting this path write it would
// let an officer certify a mail that never existed.
func TestExcusingRefusesAStateThatIsNotAnExcuse(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	for _, state := range []NoticeState{NoticeCompleted, NoticeOpen, NoticeNotRequired, NoticeQueued} {
		var verr *ValidationError
		_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
			State: state, Note: "a ground", Now: time.Now(),
		})
		if !errors.As(err, &verr) || verr.Field != fieldState {
			t.Errorf("excusing into %q answered %v, want a validation error on %q", state, err, fieldState)
		}
	}
}

// TestABlockedDutyCanStillBeExcused is the case the obstacle makes most likely,
// and the one a naive write gets wrong.
//
// Blocked says the product cannot discharge this duty by sending anything —
// which is exactly when an officer reaches for "the subject already has the
// information" or "notice is impossible". The table's blocked_shape CHECK holds
// that a reason is present exactly when the state is blocked, so an excuse that
// leaves the obstacle behind is refused by the database as a raw constraint
// violation, and the officer reads a fault rather than a refusal.
func TestABlockedDutyCanStillBeExcused(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE privacy_notice_case
		   SET state = 'blocked', blocked_reason = 'no address on file'
		 WHERE id = $1`, id); err != nil {
		t.Fatalf("blocking the case: %v", err)
	}

	got, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  "Art. 14(5)(b): no address was ever held, notice is impossible",
		Now:   time.Now(),
	})
	if err != nil {
		t.Fatalf("excusing a blocked duty: %v — the obstacle is the reason an officer "+
			"reaches for an exemption, so this is the path that matters most", err)
	}
	if got.State != NoticeExemptWithReason {
		t.Errorf("the excused case is %q, want %q", got.State, NoticeExemptWithReason)
	}
	if got.BlockedReason != nil {
		t.Errorf("the excused case still names obstacle %q: the duty has ended, and a row "+
			"asserting both that it is exempt and that something blocks it is a contradiction "+
			"the blocked_shape CHECK refuses", *got.BlockedReason)
	}
}

// TestAReassignmentRecordsWhoHadItBefore. The state does not move on a
// reassignment — it is `assigned` before and after — so an audit image of the
// state alone would record "assigned to assigned" and lose the only fact the
// write actually carries.
func TestAReassignmentRecordsWhoHadItBefore(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	var second ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO app_user (id, email, display_name)
		VALUES (gen_random_uuid(), $1, 'Second officer') RETURNING id`,
		"second-"+e.contact.String()+"@cc.test").Scan(&second); err != nil {
		t.Fatalf("seeding the second seat: %v", err)
	}

	if _, err := e.store.AssignNoticeCase(ctx, id, e.user); err != nil {
		t.Fatalf("the first assignment: %v", err)
	}
	if _, err := e.store.AssignNoticeCase(ctx, id, second); err != nil {
		t.Fatalf("the reassignment: %v", err)
	}

	var before map[string]any
	if err := e.owner.QueryRow(context.Background(), `
		SELECT before FROM audit_log
		 WHERE entity_type = 'privacy_notice_case' AND entity_id = $1
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&before); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if before[fieldOwner] != e.user.String() {
		t.Errorf("the reassignment's before-image names owner %v, want the seat it was taken "+
			"from (%v): the state does not move on a reassignment, so the owner is the only "+
			"fact the image has to carry", before[fieldOwner], e.user)
	}
}
