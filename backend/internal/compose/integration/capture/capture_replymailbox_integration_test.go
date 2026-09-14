// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Whose mailbox a reply would be joining, over the production sink.
//
// The composer offers to answer a message that arrived in somebody else's
// mailbox, and used to say nothing about whose. What it needs is the seat
// behind the delivery, and the honest source for that is capture_import — one
// row per importing mailbox, written only after the seat's own credential
// proved the message reached it. So these seed through the real sink rather
// than inserting rows: a hand-written import row would prove the reader works
// and nothing about whether capture ever writes what it reads.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// readerCtx is one seat asking a question about correspondence they may read.
func readerCtx(e *integration.SearchEnv, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestAReplyNamesTheMailboxThatTookDelivery(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	// The ticket's own shape: a workspace-shared conversation a colleague can
	// open. Held mail is the other case and has its own test below.
	setMailSharing(t, e, true)

	sync(t, customerMail("mailbox-one@acme.example"))
	activityID := oneActivityID(t, e)
	// The importing mailbox clears its own message, which is what opens it to
	// the workspace — the ticket's shape, where a colleague opens the thread
	// on a shared contact.
	setImportDecision(t, e, activityID, e.Rep1, "classified", "cleared")
	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "workspace" {
		t.Fatalf("the fixture needs a shared message, got %q", got)
	}

	store := activities.NewStore(e.DB())
	// Rep3 never had this message; Rep1's connector landed it. The question is
	// answered the same way for both, because it describes the ROW rather than
	// the asker — what changes by reader is whether they may ask at all.
	seats, err := store.MailboxesFor(readerCtx(e, e.Rep3), ids.From[ids.ActivityKind](activityID))
	if err != nil {
		t.Fatalf("asking whose mailbox took delivery: %v", err)
	}
	if len(seats) != 1 || seats[0] != e.Rep1 {
		t.Fatalf("want the delivering seat %s, got %v", e.Rep1, seats)
	}
}

func TestEveryImportingMailboxIsNamedOnce(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)

	// One message to both seats, then the SAME message again through both
	// mailboxes. A replay writes no second import row, so a seat that appeared
	// twice here would be a duplicate name in the sentence the composer prints.
	sync(t, customerMailToBoth("mailbox-two@acme.example"))
	syncSecond(t, customerMailToBoth("mailbox-two@acme.example"))
	sync(t, customerMailToBoth("mailbox-two@acme.example"))

	activityID := oneActivityID(t, e)
	store := activities.NewStore(e.DB())
	seats, err := store.MailboxesFor(readerCtx(e, e.Rep1), ids.From[ids.ActivityKind](activityID))
	if err != nil {
		t.Fatalf("asking whose mailboxes took delivery: %v", err)
	}
	if len(seats) != 2 {
		t.Fatalf("want both importing seats exactly once, got %v", seats)
	}
	seen := map[ids.UUID]bool{seats[0]: true, seats[1]: true}
	if !seen[e.Rep1] || !seen[e.Rep3] {
		t.Fatalf("want %s and %s, got %v", e.Rep1, e.Rep3, seats)
	}
}

// A hand-logged activity was typed by somebody rather than delivered to
// anybody, so there is no mailbox to name and the empty answer is the true one.
func TestAHandLoggedMessageNamesNoMailbox(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e

	store := activities.NewStore(e.DB())
	ctx := readerCtx(e, e.Rep1)
	writeCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	logged, _, err := store.LogActivity(writeCtx, activities.LogActivityInput{
		Kind: "email", Subject: strptr("Typed by a contact"), Body: strptr("Not delivered anywhere."),
	})
	if err != nil {
		t.Fatalf("logging an activity by hand: %v", err)
	}

	seats, err := store.MailboxesFor(ctx, ids.From[ids.ActivityKind](ids.UUID(logged.Id)))
	if err != nil {
		t.Fatalf("asking about a hand-logged row: %v", err)
	}
	if len(seats) != 0 {
		t.Fatalf("a hand-logged message reached nobody's mailbox, got %v", seats)
	}
}

// The provenance stamp is the older, weaker source, read only when no import
// row exists. It must name a seat this workspace still holds, and answer empty
// rather than erroring when the stored text is not a seat at all.
func TestTheProvenanceStampIsReadOnlyWhenNoImportRowStands(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	sync(t, customerMail("mailbox-legacy@acme.example"))
	activityID := oneActivityID(t, e)

	// A row as it stands BEFORE per-mailbox provenance existed: the stamp, and
	// no import row behind it.
	dropImportRows(t, e, activityID)

	store := activities.NewStore(e.DB())
	ctx := readerCtx(e, e.Rep1)
	seats, err := store.MailboxesFor(ctx, ids.From[ids.ActivityKind](activityID))
	if err != nil {
		t.Fatalf("reading the provenance stamp: %v", err)
	}
	if len(seats) != 1 || seats[0] != e.Rep1 {
		t.Fatalf("want the stamped seat %s, got %v", e.Rep1, seats)
	}

	// An EXTENSION stamps connector:ext:<unit>:<seat>, so the seat is the fourth
	// segment where a mailbox's is the third. Reading a fixed segment index
	// answers the unit name for one of these and nothing for the other.
	setCapturedBy(t, e, activityID, "connector:ext:acme-sync:"+e.Rep1.String())
	seats, err = store.MailboxesFor(ctx, ids.From[ids.ActivityKind](activityID))
	if err != nil {
		t.Fatalf("reading an extension's provenance: %v", err)
	}
	if len(seats) != 1 || seats[0] != e.Rep1 {
		t.Fatalf("want the stamped seat %s behind an extension stamp, got %v", e.Rep1, seats)
	}

	// Malformed provenance is historical data a live endpoint must survive.
	// Casting the suffix the way the backfill migration does would raise a
	// database error here and answer the caller with a 500 about old rows.
	for _, stamp := range []string{
		"connector:gmail:not-a-uuid",
		"connector:gmail:" + ids.NewV7().String(),
		"connector:gmail",
		// A trailing segment after the seat: the uuid is no longer last, so it
		// names nobody rather than attributing the row to whoever it mentions.
		"connector:gmail:" + e.Rep1.String() + ":junk",
	} {
		setCapturedBy(t, e, activityID, stamp)
		seats, err := store.MailboxesFor(ctx, ids.From[ids.ActivityKind](activityID))
		if err != nil {
			t.Fatalf("provenance %q errored instead of answering empty: %v", stamp, err)
		}
		if len(seats) != 0 {
			t.Fatalf("provenance %q named %v, want nobody", stamp, seats)
		}
	}
}

// Whose mailbox a message reached is a fact about correspondence, so it is
// answered only to a caller who may read that correspondence. A held message
// the reader can merely discover must refuse here exactly as its body does.
func TestAWithheldMessageRefusesToNameItsMailbox(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setMailSharing(t, e, false)
	sync(t, customerMail("mailbox-held@acme.example"))
	activityID := oneActivityID(t, e)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("the fixture needs a held message, got %q", got)
	}

	store := activities.NewStore(e.DB())
	// Rep3's mailbox never took this message and they are not on it, so the
	// content is not theirs — and neither is the name of the seat who has it.
	_, err := store.MailboxesFor(readerCtx(e, e.Rep3), ids.From[ids.ActivityKind](activityID))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a withheld message answered %v, want not-found", err)
	}

	// The positive control: the seat whose own mailbox delivered it still gets
	// the answer, so the refusal above is about audience and not about the
	// read being broken for everyone.
	seats, err := store.MailboxesFor(readerCtx(e, e.Rep1), ids.From[ids.ActivityKind](activityID))
	if err != nil {
		t.Fatalf("the delivering seat was refused their own message: %v", err)
	}
	if len(seats) != 1 || seats[0] != e.Rep1 {
		t.Fatalf("want %s, got %v", e.Rep1, seats)
	}
}

// dropImportRows removes the per-mailbox provenance, leaving only the stamp —
// the shape of a row captured before capture_import existed.
func dropImportRows(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`DELETE FROM capture_import WHERE activity_id = $1`, activityID)
		return err
	})
	if err != nil {
		t.Fatalf("clearing the import rows: %v", err)
	}
}

// setCapturedBy rewrites one row's provenance stamp, for the historical shapes
// the current writer can no longer produce.
func setCapturedBy(t *testing.T, e *integration.SearchEnv, activityID ids.UUID, stamp string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET captured_by = $2 WHERE id = $1`, activityID, stamp)
		return err
	})
	if err != nil {
		t.Fatalf("setting the provenance stamp: %v", err)
	}
}

func strptr(s string) *string { return &s }
