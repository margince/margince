// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A note keeps a password its member typed, while its audit images hold the
// text masked. The readers of those images compare them with the row, and must
// not read the masking as an edit.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	notePlain  = "Call notes"
	noteSecret = "Call notes\nPasswort: Hunter2-Example!"
	noteMasked = "Call notes\nPasswort: " + credentialmask.Marker
)

func noteWithBody(ctx context.Context, t *testing.T, e *integration.Env, body string) ids.UUID {
	t.Helper()
	return activityWithBody(ctx, t, e, activities.LogActivityInput{Kind: "note", Body: &body})
}

func activityWithBody(ctx context.Context, t *testing.T, e *integration.Env, in activities.LogActivityInput) ids.UUID {
	t.Helper()
	subject := "Kickoff"
	in.Subject, in.Source = &subject, "manual"
	logged, _, err := e.Activities.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("seed %s: %v", in.Kind, err)
	}
	return ids.UUID(logged.Id)
}

func storedBody(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	activity, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil || activity.Body == nil {
		t.Fatalf("read the activity back: %v", err)
	}
	return *activity.Body
}

func setNoteBody(ctx context.Context, t *testing.T, e *integration.Env, id ids.UUID, body string) {
	t.Helper()
	if _, err := e.Activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](id), activities.UpdateActivityInput{Body: &body}); err != nil {
		t.Fatalf("edit the note: %v", err)
	}
}

func TestANoteEditThatAddedAPasswordCanBePutBack(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := noteWithBody(admin, t, e, notePlain)
	setNoteBody(admin, t, e, id, noteSecret)

	entry := latestAuditRowID(t, e, "activity", id, "update")
	if got := advisoryAnswer(admin, t, e, "activity", id, entry); !got.Undoable {
		t.Errorf("an edit nobody overtook reads %q (%s), want undoable", got.Reason, got.Detail)
	}
}

// The trail, not the masked text, says whether a colleague wrote the body since:
// an edit that changed only the password leaves text that masks the same.
func TestALaterPasswordChangeStillOvertakesTheEdit(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := noteWithBody(admin, t, e, notePlain)
	setNoteBody(admin, t, e, id, noteSecret)
	entry := latestAuditRowID(t, e, "activity", id, "update")
	setNoteBody(admin, t, e, id, "Call notes\nPasswort: Another-Example-9")

	if got := advisoryAnswer(admin, t, e, "activity", id, entry); got.Undoable {
		t.Error("a later edit of the password was read as no change, so the undo would overwrite it")
	}
}

func TestANoteBodysFieldHistoryShowsThePasswordMasked(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := noteWithBody(admin, t, e, noteSecret)
	setNoteBody(admin, t, e, id, notePlain)

	field := "body"
	page, err := privacy.ListFieldHistory(admin, e.DB(), privacy.FieldHistoryFilter{EntityType: "activity", EntityID: id, Field: &field})
	if err != nil {
		t.Fatalf("reading the field history: %v", err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("the body's field history is empty; the edit should be in it")
	}
	newest := page.Entries[0]
	if newest.OldValue == nil || *newest.OldValue != noteMasked {
		t.Errorf("old value = %v, want %q", newest.OldValue, noteMasked)
	}
	if newest.NewValue == nil || *newest.NewValue != notePlain {
		t.Errorf("new value = %v, want %q", newest.NewValue, notePlain)
	}
}

// The trail keeps the password only masked, so putting the edit back would
// overwrite the note's text with the marker.
func TestANoteEditThatRemovedAPasswordCannotBePutBack(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := noteWithBody(admin, t, e, noteSecret)
	setNoteBody(admin, t, e, id, notePlain)
	entry := latestAuditRowID(t, e, "activity", id, "update")

	if got := advisoryAnswer(admin, t, e, "activity", id, entry); got.Undoable || got.Reason != string(ReasonCredentialNotKept) {
		t.Errorf("the edit reads %+v, want refused as %s", got, ReasonCredentialNotKept)
	}
	_, err := restoreSeamFor(e).Restore(admin, "activity", id, entry, currentVersion(t, e, "activity", id))
	var refused RefusedRestore
	if !errors.As(err, &refused) || refused.Reason != ReasonCredentialNotKept {
		t.Fatalf("Restore answered %v, want a %s refusal", err, ReasonCredentialNotKept)
	}
	if got := storedBody(t, e, id); got != notePlain {
		t.Errorf("the refused undo left body %q, want %q", got, notePlain)
	}
}

// A mail is stored masked, so the masked text the trail kept is what it held.
func TestAMailEditThatRemovedAPasswordCanBePutBack(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	body, direction := noteSecret, "inbound"
	id := activityWithBody(admin, t, e, activities.LogActivityInput{Kind: "email", Body: &body, Direction: &direction})
	setNoteBody(admin, t, e, id, notePlain)
	entry := latestAuditRowID(t, e, "activity", id, "update")

	if _, err := restoreSeamFor(e).Restore(admin, "activity", id, entry, currentVersion(t, e, "activity", id)); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := storedBody(t, e, id); got != noteMasked {
		t.Errorf("the undo left body %q, want %q", got, noteMasked)
	}
}

func agentConflicts(t *testing.T, e *integration.Env, id ids.UUID, body string) []string {
	t.Helper()
	patch, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := fieldOwnership{pool: e.Pool}.HumanOwnedConflicts(e.Admin(), "activity", id, patch)
	if err != nil {
		t.Fatalf("HumanOwnedConflicts: %v", err)
	}
	return conflicts
}

// The human's image holds the marker, so the marker itself is not what the
// human typed and proposing it overwrites their text.
func TestProposingTheMaskedTextOfAHumansNoteIsAConflict(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := noteWithBody(admin, t, e, notePlain)
	setNoteBody(admin, t, e, id, noteSecret)

	if got := agentConflicts(t, e, id, noteMasked); !slices.Contains(got, "body") {
		t.Errorf("conflicts = %v, want body: the proposal replaces the human's password with the marker", got)
	}
}

func TestRestatingTheTextAHumanTypedIsNoConflict(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := noteWithBody(admin, t, e, notePlain)
	setNoteBody(admin, t, e, id, noteSecret)

	if got := agentConflicts(t, e, id, noteSecret); slices.Contains(got, "body") {
		t.Errorf("conflicts = %v, want none: the proposal is the text the human typed", got)
	}
}
