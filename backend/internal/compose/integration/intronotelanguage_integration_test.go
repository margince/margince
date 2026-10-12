// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The endpoint's own correspondence-to-language path.
//
// Driven over HTTP against a real database rather than by handing the writer a
// language. A unit test that calls textlang.Detect itself and injects the result
// passes unchanged if the endpoint stops reading mail altogether, which is the
// half that was broken. What is asserted here is that German mail in the row
// reaches the note.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bootstrapSeat is the seat BootstrapWorkspace created, which is the colleague
// every route below runs through.
func bootstrapSeat(t *testing.T, e *apptest.AppEnv) ids.UUID {
	t.Helper()
	var seat ids.UUID
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE email = 'ada@example.com'`).Scan(&seat); err != nil {
		t.Fatalf("reading the bootstrap seat: %v", err)
	}
	return seat
}

// seedRoute gives the colleague a recorded edge to the contact, which is what
// makes a route exist for the note to be written for. Folded through the real
// edge recompute rather than written straight into the edge table, because a
// route the graph would not return is a fixture that proves nothing about the
// endpoint.
func seedRoute(t *testing.T, e *apptest.AppEnv, seat, contact ids.UUID) {
	t.Helper()
	ctx := context.Background()
	id := ids.NewV7()
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, audience, source, captured_by)
		VALUES ($1, 'email', 'Re: Angebot', now() - interval '3 days', 'outbound',
		        'workspace', 'manual', 'human:x')`, id); err != nil {
		t.Fatalf("seeding the exchange: %v", err)
	}
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, id, contact); err != nil {
		t.Fatalf("linking the exchange: %v", err)
	}
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'from')`,
		id, seat); err != nil {
		t.Fatalf("seeding our side: %v", err)
	}
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'to')`,
		id, contact); err != nil {
		t.Fatalf("seeding their side: %v", err)
	}
	ws := apptest.InstallationWorkspaceUUID(ctx, t, e.Pool)
	wsCtx := principal.WithWorkspaceID(ctx, ws)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, []ids.UUID{id})
	}); err != nil {
		t.Fatalf("folding the edge: %v", err)
	}
}

// seedInboundGerman plants mail the contact wrote, which is the signal the
// endpoint detects from. Their name is not prose and detects as nothing.
func seedInboundGerman(t *testing.T, e *apptest.AppEnv, contact ids.UUID, body string) {
	t.Helper()
	owner := e.Owner
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, audience, source, captured_by)
		VALUES ($1, 'email', 'Angebot Rückfrage', $2, now() - interval '2 days',
		        'inbound', 'workspace', 'manual', 'human:x')`, id, body); err != nil {
		t.Fatalf("seeding the inbound mail: %v", err)
	}
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, id, contact); err != nil {
		t.Fatalf("linking the inbound mail: %v", err)
	}
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, contact_id, role)
		VALUES ($1, $2, 'from')`, id, contact); err != nil {
		t.Fatalf("seeding the sender: %v", err)
	}
}

func TestTheIntroNoteIsWrittenInTheLanguageTheContactWroteIn(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seat := bootstrapSeat(t, e)

	var contact AnyMap
	if status := e.Call(t, "POST", "/v1/contacts",
		AnyMap{"source": "manual", "full_name": "Philipp Königs"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("creating the contact: %d %v", status, contact)
	}
	contactID, _ := contact["id"].(string)
	id := ids.MustParse(contactID)

	// The route the note is written for, and the German the note must answer in.
	seedRoute(t, e, seat, id)
	seedInboundGerman(t, e, id,
		"Guten Tag, vielen Dank für Ihre Nachricht. Wir prüfen das Angebot und melden "+
			"uns bis Ende der Woche bei Ihnen zurück. Mit freundlichen Grüßen")

	var draft AnyMap
	status := e.Call(t, "POST", "/v1/contacts/"+contactID+"/intro-note-draft",
		AnyMap{"via_user_id": seat.String()}, nil, &draft)
	if status != http.StatusOK {
		t.Fatalf("drafting the note: %d %v", status, draft)
	}
	body, _ := draft["body"].(string)

	// German, from the row. The template's German strings are the evidence: a
	// note that defaulted to English carries none of them.
	if !strings.Contains(body, "Vorstellung") && !strings.Contains(body, "Kontakt") {
		t.Errorf("a contact who writes German was written to in another language:\n%s", body)
	}
	// And the fallback notice is absent, because the language was determined.
	if undetermined, said := draft["language_undetermined"].(bool); said && undetermined {
		t.Error("the language was detected, yet the draft reports it as undetermined")
	}
}

// With nothing the contact has written, the note falls back and says so.
func TestAnIntroNoteWithNoCorrespondenceSaysTheLanguageIsUndetermined(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seat := bootstrapSeat(t, e)

	var contact AnyMap
	if status := e.Call(t, "POST", "/v1/contacts",
		AnyMap{"source": "manual", "full_name": "Brandt GmbH Kontakt"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("creating the contact: %d %v", status, contact)
	}
	contactID, _ := contact["id"].(string)

	// A route, and no inbound mail. The name alone is not prose, which is the
	// defect: "Brandt GmbH" detects as nothing.
	seedRoute(t, e, seat, ids.MustParse(contactID))

	var draft AnyMap
	if status := e.Call(t, "POST", "/v1/contacts/"+contactID+"/intro-note-draft",
		AnyMap{"via_user_id": seat.String()}, nil, &draft); status != http.StatusOK {
		t.Fatalf("drafting the note: %d %v", status, draft)
	}
	undetermined, said := draft["language_undetermined"].(bool)
	if !said || !undetermined {
		t.Errorf("a note with no correspondence behind it reports language_undetermined=%v (present=%v)",
			undetermined, said)
	}
	// Still sendable, because a language hint is not worth refusing a note over.
	if body, _ := draft["body"].(string); body == "" {
		t.Error("an undetermined language cost the rep their note")
	}
}
