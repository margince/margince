// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestArchivingTheSuggestedContactWithdrawsItsPendingProposal(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	contact := linkedInMatchFixture(ctx, t, e)
	store := contacts.NewStore(e.DB())
	svc := NewApprovalsServiceWithEffects(e.Pool)
	if _, err := store.MatchLinkedInConnections(ctx, e.Rep1); err != nil {
		t.Fatalf("matching: %v", err)
	}
	if _, err := StageLinkedInMatches(ctx, e.Pool, svc, store); err != nil {
		t.Fatalf("staging: %v", err)
	}
	if n := pendingLinkedInMatchCount(t, e); n != 1 {
		t.Fatalf("fixture: %d pending proposals, want 1", n)
	}

	if _, err := e.Contacts.ArchiveContact(ctx, ids.From[ids.ContactKind](contact), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if _, err := store.MatchLinkedInConnections(ctx, e.Rep1); err != nil {
		t.Fatalf("matching after the archive: %v", err)
	}
	staged, err := StageLinkedInMatches(ctx, e.Pool, svc, store)
	if err != nil {
		t.Fatalf("staging after the archive: %v", err)
	}

	if staged != 0 {
		t.Errorf("staged %d proposals for an archived contact, want 0", staged)
	}
	if n := pendingLinkedInMatchCount(t, e); n != 0 {
		t.Errorf("%d proposals still pending, want the dead one withdrawn", n)
	}
}

func TestMergingTheSuggestedContactReplacesItsPendingProposal(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	source := linkedInMatchFixture(ctx, t, e)
	survivor, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Andreas Survivor", Source: "manual"})
	if err != nil {
		t.Fatalf("seeding the survivor: %v", err)
	}
	store := contacts.NewStore(e.DB())
	svc := NewApprovalsServiceWithEffects(e.Pool)
	if _, err := store.MatchLinkedInConnections(ctx, e.Rep1); err != nil {
		t.Fatalf("matching: %v", err)
	}
	if _, err := StageLinkedInMatches(ctx, e.Pool, svc, store); err != nil {
		t.Fatalf("staging: %v", err)
	}

	if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](ids.UUID(survivor.Id)), nil); err != nil {
		t.Fatalf("merging: %v", err)
	}
	if _, err := StageLinkedInMatches(ctx, e.Pool, svc, store); err != nil {
		t.Fatalf("staging after the merge: %v", err)
	}

	id := onlyPendingLinkedInMatch(t, e)
	var contact ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT (proposed_change->>'contact_id')::uuid FROM approval WHERE id = $1`, id).Scan(&contact)
	}); err != nil {
		t.Fatalf("reading the proposal: %v", err)
	}
	if contact != ids.UUID(survivor.Id) {
		t.Errorf("the pending proposal names %s, want the survivor %s", contact, ids.UUID(survivor.Id))
	}
}
