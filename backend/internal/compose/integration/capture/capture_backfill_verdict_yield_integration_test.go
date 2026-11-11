// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A sender a backfill page could not resolve is queued for a verdict, and the
// verdict makes the contact later, outside the page. That contact is still the
// import's work, so the run's card counts it.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// contactVerdict answers every sender it is asked about as a contact, by the
// ref the request fenced it under.
type contactVerdict struct{}

func (contactVerdict) Complete(_ context.Context, req model.Request) (model.Response, error) {
	marker, ok := promptfence.MarkerIn(req.System)
	if !ok {
		return model.Response{}, errors.New("verdict prompt declared no data boundary")
	}
	results := []map[string]any{}
	for _, part := range strings.Split(req.Messages[0].Content, "<"+marker+` id="`)[1:] {
		ref, _, _ := strings.Cut(part, `"`)
		results = append(results, map[string]any{"id": ref, "verdict": capture.KindContact, "confidence": 0.95})
	}
	payload, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

func TestABackfillCountsTheContactTheVerdictMakesFromItsMail(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	registry.Register(&mailPageConnector{raws: [][]byte{
		email("zoe@gmail.com", "Zoe Example", captureOwner, "v1@gmail.com", ""),
	}})
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	run, err := registry.StartBackfill(grantCtx, "gmail", ids.From[ids.UserKind](e.Rep1), 6,
		connector.BackfillEstimate{Messages: 1}, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if done, _, _, err := registry.RunBackfillStep(wsCtx, run.ID); err != nil || !done {
		t.Fatalf("the page: done=%v err=%v", done, err)
	}
	if contacts, _ := readBackfillYieldColumns(t, e, run.ID); contacts != 0 {
		t.Fatalf("the page itself created %d contacts, want 0: a free-mail sender defers to the verdict", contacts)
	}

	var queuedBy *ids.UUID
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(wsCtx, `SELECT backfill_id FROM capture_pending_counterparty WHERE email = 'zoe@gmail.com'`).
			Scan(&queuedBy)
	}); err != nil {
		t.Fatalf("reading the queued sender: %v", err)
	}
	if queuedBy == nil || *queuedBy != run.ID {
		t.Fatalf("the queued sender names run %v, want %v", queuedBy, run.ID)
	}

	engine := compose.NewCounterpartyVerdictEngine(e.Pool, contactVerdict{}, compose.CaptureConfig{}, quietLog())
	if err := engine.RunWorkspace(wsCtx, 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	if contacts, _ := readBackfillYieldColumns(t, e, run.ID); contacts != 1 {
		t.Fatalf("contacts_created = %d after the verdict made the sender a contact, want 1", contacts)
	}
}
