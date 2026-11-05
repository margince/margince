// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func correctTechnicalPage(ctx context.Context, t *testing.T, e *dedupeEnv) TechnicalAddressPage {
	t.Helper()
	system := principal.SystemActing(ctx, "system:test")
	var page TechnicalAddressPage
	if err := e.store.tx(system, func(tx pgx.Tx) error {
		var err error
		page, err = e.store.RetireTechnicalAddressContactsTx(system, tx, ids.Nil, 50)
		return err
	}); err != nil {
		t.Fatalf("correcting a page: %v", err)
	}
	return page
}

func contactIsArchived(ctx context.Context, t *testing.T, e *dedupeEnv, id ids.UUID) bool {
	t.Helper()
	var archived bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM contact WHERE id = $1`, id).Scan(&archived)
	}); err != nil {
		t.Fatal(err)
	}
	return archived
}

// A contact edited between the page read and the write is read again and
// archived, not passed over: the cursor moves past it either way.
func TestATechnicalContactEditedMidPageIsStillArchived(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	logger := legacyMachineContact(ctx, t, e, "4711@bcc.hubspot.com")
	edits := 0
	beforeTechnicalWrite = func(_ context.Context, id ids.ContactID) {
		if id.UUID != logger || edits > 0 {
			return
		}
		edits++
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE contact SET version = version + 1, full_name = 'Edited meanwhile' WHERE id = $1`, logger)
			return err
		}); err != nil {
			t.Errorf("editing the contact meanwhile: %v", err)
		}
	}
	t.Cleanup(func() { beforeTechnicalWrite = nil })

	page := correctTechnicalPage(ctx, t, e)
	if edits != 1 {
		t.Fatalf("the concurrent edit ran %d times, want 1", edits)
	}
	if page.Archived != 1 || !contactIsArchived(ctx, t, e, logger) {
		t.Errorf("archived = %d, contact archived = %v; want the edited contact archived on the re-read",
			page.Archived, contactIsArchived(ctx, t, e, logger))
	}
}

// Legal hold changes nothing: a held contact keeps its technical address even
// beside a real one, and is only named for review.
func TestAMixedContactOnLegalHoldKeepsEveryAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	made, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Held Contact", Source: "manual",
		Emails: []ContactEmailInput{
			{Email: "held@holder.test", EmailType: emailTypeWork, IsPrimary: true},
			{Email: "seed-" + ids.NewV7().String() + "@holder.test", EmailType: "other", Position: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(made.Id)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE contact_email SET email = '12@bcc.hubspot.com' WHERE contact_id = $1 AND email LIKE 'seed-%'`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE contact SET legal_hold = true WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	page := correctTechnicalPage(ctx, t, e)
	if page.Retired != 0 || !slices.Contains(page.Held, id) {
		t.Errorf("retired = %d, held = %v; want the held contact named for review and nothing retired", page.Retired, page.Held)
	}
	if held := liveAddressesOf(ctx, t, e, id); !slices.Equal(held, []string{"12@bcc.hubspot.com", "held@holder.test"}) {
		t.Errorf("addresses = %v, want both kept under legal hold", held)
	}
}
