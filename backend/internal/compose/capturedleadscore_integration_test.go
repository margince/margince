// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type leadScoreRow struct {
	Score, Computed, RawSum, RoundedSum int
	Factors                             string
}

// leadScoreOf reads the lead's displayed score and its history, which must hold
// one point.
func leadScoreOf(t *testing.T, e *integration.Env, leadID ids.UUID) (displayed int, history leadScoreRow) {
	t.Helper()
	var count int
	var factors json.RawMessage
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `SELECT score FROM lead WHERE id = $1`, leadID).Scan(&displayed); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM lead_score_history WHERE lead_id = $1`, leadID).Scan(&count); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT score, score_computed, raw_sum, rounded_sum, factors
			FROM lead_score_history WHERE lead_id = $1`, leadID).
			Scan(&history.Score, &history.Computed, &history.RawSum, &history.RoundedSum, &factors)
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("the lead has %d score history rows, want 1", count)
	}
	history.Factors = string(factors)
	return displayed, history
}

// A lead minted by capture opens with the score and history point that
// CreateLead gives a hand-created lead with the same title and origin.
func TestACapturedLeadOpensWithTheScoreAHandCreatedLeadGets(t *testing.T) {
	e := integration.Setup(t)
	var origin string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT key FROM lead_origin WHERE active AND intent = 'high' ORDER BY key LIMIT 1`).Scan(&origin)
	}); err != nil {
		t.Fatal(err)
	}
	const title = "Chief Executive Officer"

	made, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Hand Made"), Title: new(title), Origin: new(origin), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(connectorCtx(e), connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "test", SourceID: "scored-1"},
		Fields:     capture.LeadFields{FullName: "Machine Made", Title: title, Origin: origin},
		Source:     "test:scored-1", CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantScore, wantHistory := leadScoreOf(t, e, ids.UUID(made.Id))
	if wantScore == 0 {
		t.Fatal("the hand-created lead scored 0, so the comparison proves nothing")
	}
	gotScore, gotHistory := leadScoreOf(t, e, ref.ID)
	if gotScore != wantScore || gotHistory != wantHistory {
		t.Errorf("captured lead scored %d with history %+v, want %d with %+v", gotScore, gotHistory, wantScore, wantHistory)
	}
}
