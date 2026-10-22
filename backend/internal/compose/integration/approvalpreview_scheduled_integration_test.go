// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestHeldMessagePreviewShowsFrozenContentAndRefusesAnUnknownPayload(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	id := p.scheduleFor(t, clocktest.Now(t).Add(2*time.Hour))
	p.withdrawConsent(t)
	p.makeDue(t, id)
	p.fire(t, id)
	card, found := p.heldCardFor(t, id)
	if !found {
		t.Fatal("held message has no decision")
	}
	read := func() crmcontracts.ApprovalPreview {
		t.Helper()
		var approval crmcontracts.Approval
		if status := p.Call(t, http.MethodGet, "/v1/approvals/"+card.ID, nil, nil, &approval); status != http.StatusOK {
			t.Fatalf("read held decision: %d", status)
		}
		if approval.Preview == nil {
			t.Fatal("held decision has no typed context")
		}
		return *approval.Preview
	}
	preview := read()
	if preview.State != "ready" || preview.Details == nil || preview.Effects == nil || !slices.Contains(*preview.Effects, "rearm_held_message") {
		t.Fatalf("held message preview=%+v", preview)
	}
	values := make(map[string]string)
	for _, detail := range *preview.Details {
		values[detail.Field] = detail.Value.Text
	}
	if values["subject"] != "Monday morning" || values["body"] != "Written the night before." || values["retry_delay_minutes"] != "15" {
		t.Fatalf("frozen content or retry effect missing: %+v", values)
	}
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE scheduled_send SET payload_version=@version WHERE id=@id`, pgx.StrictNamedArgs{"version": 999, "id": id})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	preview = read()
	if preview.State == "ready" || preview.Details != nil || preview.Effects != nil {
		t.Fatalf("unknown payload remains actionable: %+v", preview)
	}
}
