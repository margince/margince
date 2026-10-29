// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package signals

// A signal's kind and evidence can be corrected, and the audit row keeps what
// they held before. The record is seeded through CreateSignal and changed through
// UpdateSignal, because the claim is about what the real writer puts in
// audit_log.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACorrectedSignalKeepsItsOldKindAndEvidenceInTheAudit(t *testing.T) {
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	ws, admin := ids.NewV7(), ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		admin, "admin-"+admin.String()+"@signals.test"); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	store := NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)), nil)

	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + admin.String(), UserID: admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"signal": {Create: true, Read: true, Update: true}},
		},
	})

	rawRef := "mail:thread-1"
	made, err := store.CreateSignal(ctx, CreateSignalInput{
		Kind: "risk", RawRef: &rawRef, Summary: "They went quiet",
		Evidence: []crmcontracts.SignalEvidence{{Snippet: "no reply for a month"}},
	})
	if err != nil {
		t.Fatalf("seeding the signal: %v", err)
	}

	producerKind := "ghosted_thread"
	var invalid *InvalidSignalKindError
	if _, err := store.UpdateSignal(ctx, ids.From[ids.SignalKind](ids.UUID(made.Id)), UpdateSignalInput{Kind: &producerKind}); !errors.As(err, &invalid) {
		t.Fatalf("setting a producer kind returned %v, want InvalidSignalKindError", err)
	}

	kind := crmcontracts.SignalKind("buying_intent")
	evidence := []crmcontracts.SignalEvidence{{Snippet: "three unanswered mails"}}
	updated, err := store.UpdateSignal(ctx, ids.From[ids.SignalKind](ids.UUID(made.Id)), UpdateSignalInput{
		Kind: (*string)(&kind), Evidence: &evidence,
	})
	if err != nil {
		t.Fatalf("correcting the signal: %v", err)
	}
	if updated.Kind != kind || len(updated.Evidence) != 1 || updated.Evidence[0].Snippet != "three unanswered mails" {
		t.Fatalf("the signal reads kind %q evidence %v after the correction", updated.Kind, updated.Evidence)
	}

	produced := ids.New[ids.SignalKind]()
	if _, err := owner.Exec(ctx, `
		INSERT INTO signal (id, kind, source_channel, resolution_state, severity, summary,
		                    evidence, status, detected_at, source, captured_by)
		VALUES ($1, 'ghosted_thread', 'derived', 'unresolved', 'info', 'No reply',
		        '[]'::jsonb, 'open', now(), 'scan', 'test')`, produced); err != nil {
		t.Fatalf("seeding the produced signal: %v", err)
	}
	if _, err := store.UpdateSignal(ctx, produced, UpdateSignalInput{Kind: &producerKind, Evidence: &evidence}); err != nil {
		t.Errorf("echoing a produced signal's own kind while correcting its evidence returned %v, want it accepted", err)
	}

	var beforeJSON, afterJSON []byte
	if err := owner.QueryRow(ctx,
		`SELECT before, after FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1 AND action = 'update'`,
		made.Id).Scan(&beforeJSON, &afterJSON); err != nil {
		t.Fatalf("reading the correction's audit row: %v", err)
	}
	var before, after map[string]any
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatal(err)
	}
	if before["kind"] != "risk" || after["kind"] != "buying_intent" {
		t.Errorf("audit kind images are %v → %v, want risk → buying_intent", before["kind"], after["kind"])
	}
	wantBefore := []any{map[string]any{"snippet": "no reply for a month"}}
	wantAfter := []any{map[string]any{"snippet": "three unanswered mails"}}
	if !reflect.DeepEqual(before["evidence"], wantBefore) || !reflect.DeepEqual(after["evidence"], wantAfter) {
		t.Errorf("audit evidence images are %v → %v, want %v → %v", before["evidence"], after["evidence"], wantBefore, wantAfter)
	}
}
