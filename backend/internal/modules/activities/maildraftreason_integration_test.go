// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The reason a rep typed for sending anyway comes back with the draft. It moves
// only at the version the composer read, and stays off the audit trail.
func TestADraftKeepsItsSendAnywayReasonUnderTheVersionCheck(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	store := draftStore(e)

	const reason = "They asked for these terms on the phone this morning."
	content := typedDraft()
	content.OverrideExplanation = reason
	saved, err := store.SaveMailDraft(ctx, anchor, content, nil)
	if err != nil {
		t.Fatalf("saving the draft: %v", err)
	}
	got, err := store.GetMailDraft(ctx, anchor)
	if err != nil || got.Content.OverrideExplanation != reason {
		t.Fatalf("read back reason %q (%v), want %q", got.Content.OverrideExplanation, err, reason)
	}
	if resp := mailDraftResponse(got); resp.OverrideExplanation == nil || *resp.OverrideExplanation != reason {
		t.Errorf("the wire draft carries reason %v, want %q", resp.OverrideExplanation, reason)
	}

	cleared := typedDraft()
	if _, err := store.SaveMailDraft(ctx, anchor, cleared, &saved.Version); err != nil {
		t.Fatalf("saving without a reason: %v", err)
	}
	stale := typedDraft()
	stale.OverrideExplanation = "A reason written over a version that moved."
	if _, err := store.SaveMailDraft(ctx, anchor, stale, &saved.Version); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("saving a reason over a stale version → %v, want ErrVersionSkew", err)
	}
	if got, err = store.GetMailDraft(ctx, anchor); err != nil || got.Content.OverrideExplanation != "" {
		t.Errorf("after the clear and the refused save the reason reads %q (%v), want none", got.Content.OverrideExplanation, err)
	}

	tooLong := typedDraft()
	tooLong.OverrideExplanation = strings.Repeat("x", maxDraftExplanation+1)
	var invalid *InvalidMailDraftError
	if _, err := store.SaveMailDraft(ctx, anchor, tooLong, &got.Version); !errors.As(err, &invalid) || invalid.Field != fieldExplanation {
		t.Errorf("a reason past %d characters → %v, want the field refused", maxDraftExplanation, err)
	}
	if _, images := e.draftTrail(t, saved.ID); strings.Contains(images, "phone this morning") {
		t.Error("the audit trail carries the reason the rep typed")
	}
}
