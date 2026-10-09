// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Ending a manual score override over HTTP. The store's own suite clears it
// through UpdateLeadInput directly; only the door also lists the null in the
// request's named clears, and that list is what refused the gesture.

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type leadScoreWire struct {
	ID                  string  `json:"id"`
	Score               int     `json:"score"`
	ScoreComputed       *int    `json:"score_computed"`
	ScoreOverrideReason *string `json:"score_override_reason"`
	Version             int64   `json:"version"`
}

func TestANullScoreEndsTheOverrideOverHTTP(t *testing.T) {
	for _, field := range []string{"score", "score_override_reason"} {
		t.Run(field, func(t *testing.T) {
			e := apptest.SetupApp(t)
			e.BootstrapWorkspace(t)
			lead, base := overriddenLead(t, e)

			var cleared leadScoreWire
			if status := e.Call(t, "PATCH", base, map[string]any{field: nil},
				map[string]string{"If-Match": strconv.FormatInt(lead.Version, 10)}, &cleared); status != http.StatusOK {
				t.Fatalf("PATCH %s: null -> %d, want 200", field, status)
			}
			if cleared.ScoreOverrideReason != nil || cleared.ScoreComputed != nil {
				t.Errorf("override survived its clear: reason=%v computed=%v",
					cleared.ScoreOverrideReason, cleared.ScoreComputed)
			}
			if cleared.Score == lead.Score {
				t.Errorf("score stayed at the human's %d; it must fall back to the machine value", lead.Score)
			}
			assertOverrideClearAudited(t, e, lead.ID)
		})
	}
}

// overriddenLead creates a lead and sets a manual score on it that the
// machine score cannot equal, so a fall-back is visible in the number.
func overriddenLead(t *testing.T, e *apptest.AppEnv) (leadScoreWire, string) {
	t.Helper()
	var lead leadScoreWire
	if status := e.Call(t, "POST", "/v1/leads", map[string]any{
		"full_name": "Sasha Score", "email": "sasha@score.test", "source": "manual",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead -> %d, want 201", status)
	}
	base := "/v1/leads/" + lead.ID
	manual := 99
	if lead.Score == manual {
		manual = 1
	}
	var overridden leadScoreWire
	if status := e.Call(t, "PATCH", base, map[string]any{
		"score": manual, "score_override_reason": "board-level sponsor",
	}, nil, &overridden); status != http.StatusOK {
		t.Fatalf("set override -> %d, want 200", status)
	}
	if overridden.ScoreOverrideReason == nil || overridden.Score != manual {
		t.Fatalf("override not in force: %+v", overridden)
	}
	return overridden, base
}

// assertOverrideClearAudited reads the audit row the clear wrote and checks it
// holds the reason in the before image and no reason in the after image.
func assertOverrideClearAudited(t *testing.T, e *apptest.AppEnv, leadID string) {
	t.Helper()
	var before, after json.RawMessage
	err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT before, after FROM audit_log
			  WHERE entity_type = 'lead' AND entity_id = $1
			    AND before ? 'score_override_reason' AND after ? 'score_override_reason'
			    AND after->'score_override_reason' = 'null'::jsonb
			  ORDER BY occurred_at DESC LIMIT 1`, leadID).Scan(&before, &after)
	})
	if err != nil {
		t.Fatalf("no audit row records the clear: %v", err)
	}
	var was map[string]any
	if err := json.Unmarshal(before, &was); err != nil {
		t.Fatalf("before image: %v", err)
	}
	if was["score_override_reason"] != "board-level sponsor" {
		t.Errorf("before image lost the reason: %s", before)
	}
}
