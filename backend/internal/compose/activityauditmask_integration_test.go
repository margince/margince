// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A note keeps a password its member typed, while its audit images hold the
// text masked. The readers of those images compare them with the row, and must
// not read the masking as an edit.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
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
	subject := "Kickoff"
	note, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: &subject, Body: &body, Source: "manual"})
	if err != nil {
		t.Fatalf("seed note: %v", err)
	}
	return ids.UUID(note.Id)
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
