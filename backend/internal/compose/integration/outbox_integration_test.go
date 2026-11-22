// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The write-shape invariant on the wire (B-EP04.5): an authenticated HTTP
// write stages exactly one outbox envelope, complete per events.md §2 —
// server-minted event_id/correlation_id, the actor from the session (never
// the request body), and trace.audit_log_id pointing at the audit row the
// same transaction committed.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// stagedEnvelope is one event_outbox row a write staged.
type stagedEnvelope struct {
	stream string
	env    events.Envelope
}

// stagedContactCreated reads the contact.created rows out of the outbox,
// asserting the single HTTP mutation staged exactly one. Bootstrap
// itself stages config events (pipeline.created from the C5 seed); the
// write-shape assertion is about the CONTACT mutation alone.
func stagedContactCreated(t *testing.T, owner *pgx.Conn) stagedEnvelope {
	t.Helper()
	rows, err := owner.Query(t.Context(),
		`SELECT stream, envelope FROM event_outbox WHERE envelope->>'type' = 'contact.created' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stagedEnvelope, error) {
		var s stagedEnvelope
		var raw []byte
		if err := row.Scan(&s.stream, &raw); err != nil {
			return s, err
		}
		return s, json.Unmarshal(raw, &s.env)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("one mutation staged %d contact.created rows, want exactly 1", len(all))
	}
	return all[0]
}

func TestWriteStagesOneCompleteEnvelope(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var me AnyMap
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("/me status = %d", status)
	}

	var contact AnyMap
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"source":    "manual",
		"full_name": "Grace Hopper",
		"emails":    []AnyMap{{"email": "grace@example.com"}},
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact status = %d, body %v", status, contact)
	}

	owner, err := pgx.Connect(t.Context(), os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})

	got := stagedContactCreated(t, owner)
	if got.stream != "gw:events:crm:contact" {
		t.Errorf("staged on %s, want gw:events:crm:contact", got.stream)
	}
	if err := got.env.Validate(); err != nil {
		t.Errorf("staged envelope fails its own contract: %v", err)
	}
	if got.env.Type != "contact.created" || got.env.Version != 1 {
		t.Errorf("type/version = %s/%d, want contact.created/1", got.env.Type, got.env.Version)
	}
	contactID, _ := contact["id"].(string)
	if got.env.Entity.Type != "contact" || got.env.Entity.ID.String() != contactID {
		t.Errorf("entity ref %+v does not name the created contact %v", got.env.Entity, contactID)
	}
	adminID, _ := me["user"].(map[string]any)["id"].(string)
	if got.env.Actor.Type != "human" || got.env.Actor.ID != "human:"+adminID {
		t.Errorf("actor %+v is not the authenticated admin %q (provenance must come from the session)", got.env.Actor, adminID)
	}
	if got.env.Trace.CausationID != nil {
		t.Error("a direct API write starts a chain; causation_id must be null")
	}

	// The trace must point at the audit row the same transaction wrote.
	var auditAction, auditEntity string
	err = owner.QueryRow(t.Context(),
		`SELECT action, entity_type FROM audit_log WHERE id = $1`,
		got.env.Trace.AuditLogID).Scan(&auditAction, &auditEntity)
	if err != nil {
		t.Fatalf("trace.audit_log_id %s resolves to no audit row: %v", got.env.Trace.AuditLogID, err)
	}
	if auditAction != "create" || auditEntity != "contact" {
		t.Errorf("linked audit row is %s/%s, want create/contact", auditAction, auditEntity)
	}
}

func TestFailedLoginIsAuditedAndThrottled(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Fable E2E", "ada@example.com", "Admin")

	if status := e.Call(t, "POST", "/v1/auth/login", AnyMap{
		"email": "ada@example.com", "password": "wrong-password-entirely",
	}, nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d, want 401", status)
	}

	owner, err := pgx.Connect(t.Context(), os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	var failed int
	if err := owner.QueryRow(t.Context(),
		`SELECT count(*) FROM system_log WHERE action = 'login' AND detail->>'outcome' = 'failed'`).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if failed != 1 {
		t.Fatalf("failed-login system_log rows = %d, want 1", failed)
	}

	// Failures two through five stay 401; the fifth consecutive failure
	// locks the account (formulas §27). A locked account then answers 401 —
	// INDISTINGUISHABLE from bad credentials (F-005), even for the CORRECT
	// password — with a decoy Argon2 pass for timing parity, so it is not an
	// account-existence oracle. The in-process 429 flood throttle sits in
	// front for distinct-email sprays; on a single account the lock wins.
	for i := 2; i <= 5; i++ {
		if status := e.Call(t, "POST", "/v1/auth/login", AnyMap{
			"email": "ada@example.com", "password": "wrong-password-entirely",
		}, nil, nil); status != http.StatusUnauthorized {
			t.Fatalf("failure %d = %d, want 401", i, status)
		}
	}
	if status := e.Call(t, "POST", "/v1/auth/login", AnyMap{
		"email": "ada@example.com", "password": "wrong-password-entirely",
	}, nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("attempt past the lock threshold = %d, want 401 (a locked account reads as bad credentials)", status)
	}
	if status := e.Call(t, "POST", "/v1/auth/login", AnyMap{
		"email": "ada@example.com", "password": "correct-horse-battery",
	}, nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("correct password on a locked account = %d, want 401 (indistinguishable from an unknown email)", status)
	}
	var lockouts int
	if err := owner.QueryRow(t.Context(),
		`SELECT count(*) FROM system_log WHERE action = 'login' AND detail->>'outcome' = 'lockout'`).Scan(&lockouts); err != nil {
		t.Fatal(err)
	}
	if lockouts != 1 {
		t.Fatalf("lockout system_log rows = %d, want 1", lockouts)
	}
}

// A write's response names the request in X-Request-Id. That id is the
// correlation_id of the event the same transaction staged beside its audit row.
// Server-Timing rides the signed-in answer only: a failed login, which no
// session answers, carries the id and no timing.
func TestResponseRequestIDJoinsTheWriteItCaused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	post := func(path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.TS.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		apptest.CloseBody(t, resp)
		return resp
	}

	created := post("/v1/contacts", `{"source":"manual","full_name":"Grace Hopper","emails":[{"email":"grace@example.com"}]}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create contact status = %d", created.StatusCode)
	}
	if !strings.HasPrefix(created.Header.Get("Server-Timing"), "total;dur=") {
		t.Errorf("signed-in answer Server-Timing = %q, want total;dur=<ms>", created.Header.Get("Server-Timing"))
	}

	owner, err := pgx.Connect(t.Context(), os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	staged := stagedContactCreated(t, owner)
	if got := created.Header.Get("X-Request-Id"); got == "" || got != staged.env.Trace.CorrelationID.String() {
		t.Errorf("X-Request-Id %q, staged event correlation_id %s: want the same id", got, staged.env.Trace.CorrelationID)
	}

	refused := post("/v1/auth/login", `{"email":"nobody@example.com","password":"wrong-password-entirely"}`)
	if refused.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d, want 401", refused.StatusCode)
	}
	if refused.Header.Get("X-Request-Id") == "" {
		t.Error("a 401 carries no X-Request-Id")
	}
	if got := refused.Header.Get("Server-Timing"); got != "" {
		t.Errorf("failed login Server-Timing = %q, want none", got)
	}
}
