// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Putting back a lead's source change. The change rescores the lead, and the
// score is derived from the source, so the restore sends the source alone and
// the rescore follows it.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type leadScoreRecord struct {
	ID            string `json:"id"`
	Version       int64  `json:"version"`
	Source        string `json:"source"`
	Score         int    `json:"score"`
	ScoreComputed *int   `json:"score_computed"`
}

func readLeadScore(t *testing.T, e *apptest.AppEnv, id string) leadScoreRecord {
	t.Helper()
	var lead leadScoreRecord
	if status := e.Call(t, "GET", "/v1/leads/"+id, nil, nil, &lead); status != 200 {
		t.Fatalf("read lead → %d", status)
	}
	return lead
}

func createManualLead(t *testing.T, e *apptest.AppEnv) leadScoreRecord {
	t.Helper()
	var created leadScoreRecord
	if status := e.Call(t, "POST", "/v1/leads",
		AnyMap{"full_name": "Jonas Petersen", "title": "VP Sales", "source": "manual"}, nil, &created); status != 201 {
		t.Fatalf("create lead → %d", status)
	}
	return created
}

func TestEndToEnd_aLeadSourceChangeThatMovedTheScoreGoesBack(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	created := createManualLead(t, e)

	if status := e.Call(t, "PATCH", "/v1/leads/"+created.ID, AnyMap{"source": "referral"}, nil, nil); status != 200 {
		t.Fatalf("change the source → %d", status)
	}
	changed := readLeadScore(t, e, created.ID)
	if changed.Score == created.Score {
		t.Fatalf("premise: the source change must move the score, both %d", created.Score)
	}

	entry := theUpdateEntry(t, readHistory(t, e, "lead", created.ID))
	if !entry.Undoable.Undoable {
		t.Fatalf("a fresh source change reads as not undoable: %s", reasonOf(entry))
	}
	if status, _ := restore(t, e, "lead", created.ID, entry.ID, changed.Version); status != 200 {
		t.Fatalf("restore → %d, want 200", status)
	}
	back := readLeadScore(t, e, created.ID)
	if back.Source != "manual" || back.Score != created.Score {
		t.Errorf("after the restore source=%q score=%d, want manual and %d", back.Source, back.Score, created.Score)
	}
}

// Under a Commercial Judgement override the rescore moves the machine value and
// leaves the human's number alone, so the restore must do the same.
func TestEndToEnd_aLeadSourceChangeUnderAnOverrideGoesBack(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	created := createManualLead(t, e)

	if status := e.Call(t, "PATCH", "/v1/leads/"+created.ID,
		AnyMap{"score": 88, "score_override_reason": "strategic account"}, nil, nil); status != 200 {
		t.Fatalf("set the override → %d", status)
	}
	overridden := readLeadScore(t, e, created.ID)
	if status := e.Call(t, "PATCH", "/v1/leads/"+created.ID, AnyMap{"source": "referral"}, nil, nil); status != 200 {
		t.Fatalf("change the source → %d", status)
	}
	changed := readLeadScore(t, e, created.ID)
	if changed.ScoreComputed == nil || *changed.ScoreComputed == *overridden.ScoreComputed {
		t.Fatalf("premise: the machine score must follow the source, %v then %v", overridden.ScoreComputed, changed.ScoreComputed)
	}

	entry := theUpdateEntry(t, readHistory(t, e, "lead", created.ID))
	if status, _ := restore(t, e, "lead", created.ID, entry.ID, changed.Version); status != 200 {
		t.Fatalf("restore → %d, want 200", status)
	}
	back := readLeadScore(t, e, created.ID)
	if back.Source != "manual" || back.Score != 88 {
		t.Errorf("after the restore source=%q score=%d, want manual and the human's 88", back.Source, back.Score)
	}
	if back.ScoreComputed == nil || *back.ScoreComputed != *overridden.ScoreComputed {
		t.Errorf("machine score after the restore = %v, want %v", back.ScoreComputed, overridden.ScoreComputed)
	}
}
