// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// What the mail claim refuses, against a real database.
//
// The lane that drives it is proven end to end in compose, where the sender
// lives. What cannot be reached from there is the pair of refusals underneath:
// a cause offered for a notice nobody claimed, and one offered for a notice
// that does not exist. Both are the same statement declining to write, and the
// table's own CHECK says the same thing from the other side — a cause may not
// stand beside an unspent claim.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A cause is only ever written beside a claim that was actually taken.
//
// Not a defensive check: EmailFailed is what the sender calls when the relay
// refuses, and calling it on an unclaimed notice would mean the sender had
// recorded a failed send for a message it never attempted. The refusal is what
// turns that into an error somebody sees rather than a row that quietly
// contradicts the column beside it.
func TestACauseIsRefusedWithoutTheClaimItExplains(t *testing.T) {
	t.Parallel()
	e := setupNotices(t)
	id, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient,
		Kind:      "automation",
		Subject:   "A deal you own moved stage",
	})
	if err != nil {
		t.Fatalf("recording the notice: %v", err)
	}

	if err := e.store.EmailFailed(e.engineCtx(), id, "relay refused the recipient"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("recording a cause on an unattempted notice → %v, want %v", err, apperrors.ErrNotFound)
	}

	var cause *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT email_error FROM notice WHERE id = $1`, id).Scan(&cause); err != nil {
		t.Fatalf("reading the notice back: %v", err)
	}
	if cause != nil {
		t.Errorf("the refusal still wrote a cause beside no attempt: %q", *cause)
	}

	// The claim taken, the same call lands — so the refusal above is about the
	// missing claim and not about EmailFailed being unable to write at all.
	if _, claimed, err := e.store.ClaimEmailAttempt(e.engineCtx(), id); err != nil || !claimed {
		t.Fatalf("claiming the attempt → claimed=%v, err=%v", claimed, err)
	}
	if err := e.store.EmailFailed(e.engineCtx(), id, "relay refused the recipient"); err != nil {
		t.Fatalf("recording the cause on a claimed notice: %v", err)
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT email_error FROM notice WHERE id = $1`, id).Scan(&cause); err != nil {
		t.Fatalf("reading the notice back: %v", err)
	}
	if cause == nil || *cause != "relay refused the recipient" {
		t.Errorf("the claimed notice does not carry the cause: %v", cause)
	}
}

// A notice that is not there answers absent rather than pretending to record.
//
// The same branch as the case above, reached the other way: an id that names no
// row. It matters because the sender is handed an id by a job, and a job can
// outlive the notice it names — an erasure, a purge — so "nothing to record" has
// to be an answer rather than a silent success.
func TestACauseForANoticeThatIsGoneReportsItAbsent(t *testing.T) {
	t.Parallel()
	e := setupNotices(t)

	err := e.store.EmailFailed(e.engineCtx(), ids.NewV7(), "relay refused the recipient")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("recording a cause on a notice that is gone → %v, want %v", err, apperrors.ErrNotFound)
	}
}

// A release frees every claim of the send or none. Three claims, one of them
// taken again since: nothing is released and the caller is told.
func TestAReleaseWithOneChangedClaimReleasesNone(t *testing.T) {
	t.Parallel()
	e := setupNotices(t)
	var attempts []EmailAttempt
	for _, subject := range []string{"First", "Second", "Third"} {
		id, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "automation", Subject: subject})
		if err != nil {
			t.Fatalf("recording the notice: %v", err)
		}
		attempt, claimed, err := e.store.ClaimEmailAttempt(e.engineCtx(), id)
		if err != nil || !claimed {
			t.Fatalf("claiming %s → claimed=%v, err=%v", subject, claimed, err)
		}
		attempts = append(attempts, attempt)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE notice SET email_attempted_at = email_attempted_at + interval '1 second' WHERE id = $1`,
		attempts[1].Notice.ID); err != nil {
		t.Fatalf("moving one claim: %v", err)
	}

	if err := e.store.ReleaseEmailAttempts(e.engineCtx(), attempts); !errors.Is(err, ErrReleaseChanged) {
		t.Fatalf("releasing with one changed claim → %v, want %v", err, ErrReleaseChanged)
	}
	var held int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM notice WHERE id = ANY($1::uuid[]) AND email_attempted_at IS NOT NULL`,
		[]ids.UUID{attempts[0].Notice.ID, attempts[1].Notice.ID, attempts[2].Notice.ID}).Scan(&held); err != nil {
		t.Fatalf("reading the claims back: %v", err)
	}
	if held != 3 {
		t.Fatalf("%d of 3 claims are still held after a refused release, want all 3", held)
	}

	// The unchanged two, one named twice, are freed together.
	if err := e.store.ReleaseEmailAttempts(e.engineCtx(), []EmailAttempt{attempts[0], attempts[2], attempts[0]}); err != nil {
		t.Fatalf("releasing two unchanged claims: %v", err)
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM notice WHERE id = ANY($1::uuid[]) AND email_attempted_at IS NOT NULL`,
		[]ids.UUID{attempts[0].Notice.ID, attempts[2].Notice.ID}).Scan(&held); err != nil {
		t.Fatalf("reading the claims back: %v", err)
	}
	if held != 0 {
		t.Fatalf("%d claims are still held after a release of unchanged claims", held)
	}
}

// A notice its producer took back is not mailed, even when its job runs later.
func TestARetractedNoticeIsNotMailed(t *testing.T) {
	t.Parallel()
	e := setupNotices(t)
	id, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: KindMeetingTeamRequest, Subject: "Ada added you to the booking link", DedupeKey: "team:retracted"})
	if err != nil {
		t.Fatalf("recording the notice: %v", err)
	}
	if err := e.store.db.Tx(e.engineCtx(), func(tx pgx.Tx) error {
		_, err := e.store.Retract(e.engineCtx(), tx, "team:retracted")
		return err
	}); err != nil {
		t.Fatalf("taking the notice back: %v", err)
	}
	if _, claimed, err := e.store.ClaimEmailAttempt(e.engineCtx(), id); err != nil || claimed {
		t.Fatalf("claiming a retracted notice's mail → claimed=%v, err=%v; want nothing claimed", claimed, err)
	}
}

// A burst claims the recipient's other live approval mails and leaves a
// retracted one unclaimed.
func TestABurstLeavesARetractedApprovalUnclaimed(t *testing.T) {
	t.Parallel()
	e := setupNotices(t)
	keys := []string{ApprovalNoticeKey(ids.NewV7()), ApprovalNoticeKey(ids.NewV7()), ApprovalNoticeKey(ids.NewV7())}
	notices := make([]ids.UUID, len(keys))
	for i, key := range keys {
		id, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: KindApprovalPending, Subject: "Approve " + key, DedupeKey: key})
		if err != nil {
			t.Fatalf("recording the notice: %v", err)
		}
		notices[i] = id
	}
	if err := e.store.db.Tx(e.engineCtx(), func(tx pgx.Tx) error {
		_, err := e.store.Retract(e.engineCtx(), tx, keys[2])
		return err
	}); err != nil {
		t.Fatalf("taking one approval back: %v", err)
	}
	first, claimed, err := e.store.ClaimEmailAttempt(e.engineCtx(), notices[0])
	if err != nil || !claimed {
		t.Fatalf("claiming the first mail → claimed=%v, err=%v", claimed, err)
	}
	burst, err := e.store.ClaimApprovalBurst(e.engineCtx(), first, time.Hour)
	if err != nil {
		t.Fatalf("claiming the burst: %v", err)
	}
	if len(burst) != 1 || burst[0].Notice.ID != notices[1] {
		t.Fatalf("the burst claimed %+v, want only the live approval", burst)
	}
	var attempted *time.Time
	if err := e.owner.QueryRow(context.Background(), `SELECT email_attempted_at FROM notice WHERE id = $1`, notices[2]).Scan(&attempted); err != nil || attempted != nil {
		t.Fatalf("the retracted approval's mail was claimed: %v %v", attempted, err)
	}
}

// A reword gives a live line new words and leaves whether it was read alone. A
// line taken back is left as it is.
func TestARewordChangesOnlyALiveLine(t *testing.T) {
	t.Parallel()
	e := setupNotices(t)
	for _, key := range []string{"team:live", "team:retracted"} {
		if _, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: KindMeetingTeamRequest, Subject: "Added to “Intro call”", DedupeKey: key}); err != nil {
			t.Fatalf("recording the notice: %v", err)
		}
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE notice SET read_at = now() WHERE dedupe_key = 'team:live'`); err != nil {
		t.Fatal(err)
	}
	if err := e.store.db.Tx(e.engineCtx(), func(tx pgx.Tx) error {
		if _, err := e.store.Retract(e.engineCtx(), tx, "team:retracted"); err != nil {
			return err
		}
		for _, key := range []string{"team:live", "team:retracted"} {
			if err := e.store.RewordLiveTx(e.engineCtx(), tx, NewNotice{Recipient: e.recipient, Kind: KindMeetingTeamRequest, Subject: "Added to “Renewal call”", DedupeKey: key}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("rewording: %v", err)
	}
	for key, want := range map[string]string{"team:live": "Added to “Renewal call”", "team:retracted": "Added to “Intro call”"} {
		var subject string
		var read bool
		if err := e.owner.QueryRow(context.Background(), `SELECT subject, read_at IS NOT NULL FROM notice WHERE dedupe_key = $1`, key).Scan(&subject, &read); err != nil {
			t.Fatal(err)
		}
		if subject != want || read != (key == "team:live") {
			t.Errorf("%s reads %q (read %v), want %q with its read state kept", key, subject, read, want)
		}
	}
}
