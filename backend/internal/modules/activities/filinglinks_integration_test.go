// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A task made from a message is filed where the message is. A company the
// message does not hold, that the caller may not change, is left out, and the
// task keeps the rest. Any other record the caller may not change refuses it.
func TestATaskFromAMessageDropsOnlyAGuessedCompanyItMayNotChange(t *testing.T) {
	e := setupSend(t)
	message := e.seedAnchor(t, "parent@buyer.test", "root@buyer.test")
	carried := e.colleaguesCompany(t, message)
	guessed := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO company (id, display_name, owner_id, source, captured_by)
		 VALUES ($1, 'Guessed GmbH', $2, 'manual', 'human:x')`, guessed, e.other); err != nil {
		t.Fatalf("seeding the guessed company: %v", err)
	}
	contact := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		 VALUES ($1, 'Colleague contact', $2, 'manual', 'human:x')`, contact, e.other); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	store := e.store(stubUnsubscribeLinker{})
	ctx := e.as(principal.RowScopeOwn)
	onCarried := ActivityLinkInput{EntityType: "company", EntityID: carried}

	kept, err := store.FilingLinks(ctx, message.UUID, []ActivityLinkInput{onCarried, {EntityType: "company", EntityID: guessed}})
	if err != nil {
		t.Fatalf("filing links with a guessed company → %v, want the company left out", err)
	}
	if !slices.Equal(kept, []ActivityLinkInput{onCarried}) {
		t.Fatalf("kept %v, want only the company the message is filed on", kept)
	}
	if _, err := store.FilingLinks(ctx, message.UUID, []ActivityLinkInput{onCarried, {EntityType: "contact", EntityID: contact}}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("filing links with a colleague's contact the message does not hold → %v, want permission denied", err)
	}
}
