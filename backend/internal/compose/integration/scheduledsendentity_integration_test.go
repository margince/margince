// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A held scheduled message previews the sign-off of the legal entity it was
// scheduled for, which its fire also uses. That holds after its sender has
// moved to another entity.
//
// Mutation: drop in.SendingEntity from the preview's PreviewMailSignOff call
// and the preview signs as the default entity.
func TestAHeldMessagePreviewsTheSignatureOfItsFrozenEntity(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	branch := ids.NewV7()
	p.inWorkspace(t, `INSERT INTO legal_entity (id, display_name, legal_name, country, imprint, signature_template, captured_by)
		VALUES ($1, 'Branch', 'Branch GmbH', 'at', 'Branch GmbH imprint', '<p>{name}</p><p>{imprint}</p>', 'human:x')`, branch)
	p.inWorkspace(t, `INSERT INTO legal_entity_member (legal_entity_id, user_id, assigned_by)
		VALUES ($1, $2, 'human:x')`, branch, p.user)

	marketing := p.scheduleClaiming(t, AnyMap{"communication_context": "marketing"})
	stop := compose.NewScheduledSendStop(p.Pool, identity.NewService(p.Pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := stop.HandleEvent(context.Background(), kevents.Envelope{
		EventID: ids.NewV7(), Type: "consent.suppressed", Version: 1, OccurredAt: clocktest.Now(t).UTC(),
		Entity:  kevents.EntityRef{Type: "contact", ID: uuidOf(t, p.contactID)},
		Payload: []byte(`{"kind":"objection","decided_by_level":"subject"}`),
	}); err != nil {
		t.Fatalf("handling the stop: %v", err)
	}
	card, found := p.heldCardFor(t, marketing)
	if !found {
		t.Fatal("the stopped message raised no card")
	}
	p.inWorkspace(t, `DELETE FROM legal_entity_member WHERE user_id = $1`, p.user)

	var approval struct {
		Preview *struct {
			Details []struct {
				Field string `json:"field"`
				Value struct {
					Text string `json:"text"`
				} `json:"value"`
			} `json:"details"`
		} `json:"preview"`
	}
	if status := p.Call(t, "GET", "/v1/approvals/"+card.ID, nil, nil, &approval); status != http.StatusOK {
		t.Fatalf("reading the card → %d", status)
	}
	if approval.Preview == nil {
		t.Fatal("the card has no preview")
	}
	signed := ""
	for _, detail := range approval.Preview.Details {
		if detail.Field == "signature_text" {
			signed = detail.Value.Text
		}
	}
	if !strings.Contains(signed, "Branch GmbH imprint") {
		t.Errorf("the held message previews the sign-off %q, want the frozen entity's imprint", signed)
	}
}

// inWorkspace runs one statement in the fixture's workspace.
func (p *preflightEnv) inWorkspace(t *testing.T, sql string, args ...any) {
	t.Helper()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	}); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}
