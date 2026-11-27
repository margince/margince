// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A bulk stage move through both bulk doors. Each deal goes through the
// single-record advance, which writes a history row, an audit row and a
// deal.stage_changed event. The undo is a second move and keeps the first
// move's history and event. A rule of the advance refuses one deal at a time
// and does not stop the others.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// openStagePair answers the seeded pipeline's first two open stages.
func openStagePair(t *testing.T, e *apptest.AppEnv) (pipeline, first, second string) {
	t.Helper()
	var listing struct {
		Data []struct {
			ID     string `json:"id"`
			Stages []struct {
				ID       string `json:"id"`
				Semantic string `json:"semantic"`
				Position int    `json:"position"`
			} `json:"stages"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &listing); status != http.StatusOK || len(listing.Data) == 0 {
		t.Fatalf("reading the pipelines → %d", status)
	}
	best := listing.Data[0]
	var open []string
	for position := range 100 {
		for _, stage := range best.Stages {
			if stage.Semantic == "open" && stage.Position == position {
				open = append(open, stage.ID)
			}
		}
	}
	if len(open) < 2 {
		t.Fatalf("the seeded pipeline has %d open stages, want at least two", len(open))
	}
	return best.ID, open[0], open[1]
}

// seedStageDeals creates n open deals on stage and answers each as a list row
// would carry it.
func seedStageDeals(t *testing.T, e *apptest.AppEnv, pipeline, stage string, n int) []bulkItemDTO {
	t.Helper()
	items := make([]bulkItemDTO, 0, n)
	for range n {
		var created bulkItemDTO
		if status := e.Call(t, "POST", "/v1/deals", AnyMap{
			"source": "manual", "name": "Bulk stage deal " + ids.NewV7().String(),
			"pipeline_id": pipeline, "stage_id": stage,
		}, nil, &created); status != http.StatusCreated {
			t.Fatalf("seeding a deal → %d", status)
		}
		items = append(items, created)
	}
	return items
}

func moveDeals(stage string, items []bulkItemDTO) AnyMap {
	return AnyMap{"record_type": "deal", "verb": "move_stage", "stage_id": stage, "items": items}
}

func stageOfDeal(t *testing.T, e *apptest.AppEnv, id string) string {
	t.Helper()
	var stage string
	if err := e.Owner.QueryRow(t.Context(), `SELECT stage_id::text FROM deal WHERE id = $1`, id).Scan(&stage); err != nil {
		t.Fatalf("reading the stage of deal %s: %v", id, err)
	}
	return stage
}

func stageChangedEvents(t *testing.T, e *apptest.AppEnv, deal string) int {
	t.Helper()
	return countOwned(t, e, `SELECT count(*) FROM event_outbox
		WHERE envelope::text LIKE '%deal.stage_changed%' AND envelope::text LIKE '%' || $1 || '%'`, deal)
}

func TestBothDoorsMoveDealsToAStageAndTheUndoMovesThemBackBesideTheFirstMove(t *testing.T) {
	t.Parallel()
	e, doors := bulkUndoApp(t, "bulk-stage")
	pipeline, from, to := openStagePair(t, e)
	for _, door := range doors {
		items := seedStageDeals(t, e, pipeline, from, 3)
		out, refusal := door.execute(t, moveDeals(to, items), "")
		if refusal != "" || out.Changed != 3 {
			t.Fatalf("%s: moving three deals → %+v, refused %q", door.name, out, refusal)
		}
		for _, item := range items {
			if got := stageOfDeal(t, e, item.ID); got != to {
				t.Errorf("%s: deal %s stands on %s after the move, want %s", door.name, item.ID, got, to)
			}
			if n := stageChangedEvents(t, e, item.ID); n != 1 {
				t.Errorf("%s: deal %s has %d deal.stage_changed events after the move, want one", door.name, item.ID, n)
			}
		}
		if n := countOwned(t, e, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND entity_type = 'deal'`, out.BatchID); n != 3 {
			t.Errorf("%s: %d deal audit rows carry the batch id, want three", door.name, n)
		}

		undone, refusal := door.undo(t, out.BatchID, "")
		if refusal != "" || undone.Changed != 3 || undone.UndoOf != out.BatchID {
			t.Fatalf("%s: undo → %+v, refused %q", door.name, undone, refusal)
		}
		for _, item := range items {
			if got := stageOfDeal(t, e, item.ID); got != from {
				t.Errorf("%s: deal %s stands on %s after the undo, want %s", door.name, item.ID, got, from)
			}
			if n := stageChangedEvents(t, e, item.ID); n != 2 {
				t.Errorf("%s: deal %s has %d deal.stage_changed events after the undo, want the move's and the undo's", door.name, item.ID, n)
			}
			// The first move's history row stays, and the undo's row names it.
			if n := countOwned(t, e, `SELECT count(*) FROM deal_stage_history
				WHERE deal_id = $1 AND to_stage_id::text = $2 AND reversal_of IS NULL`, item.ID, to); n != 1 {
				t.Errorf("%s: deal %s keeps %d history rows of the first move, want one", door.name, item.ID, n)
			}
			if n := countOwned(t, e, `SELECT count(*) FROM deal_stage_history h
				WHERE h.deal_id = $1 AND h.reversal_of IN (SELECT id FROM deal_stage_history WHERE deal_id = $1 AND to_stage_id::text = $2)`,
				item.ID, to); n != 1 {
				t.Errorf("%s: %d history rows of deal %s reverse the move, want one", door.name, n, item.ID)
			}
		}
		if _, refusal := door.undo(t, out.BatchID, ""); !strings.Contains(refusal, "already undone") {
			t.Errorf("%s: a second undo → %q, want it refused as already undone", door.name, refusal)
		}
	}
}

func TestBothDoorsLeaveAlonePerDealWhatTheAdvanceRefusesAndSayWhy(t *testing.T) {
	t.Parallel()
	e, doors := bulkUndoApp(t, "bulk-stage-skips")
	pipeline, from, to := openStagePair(t, e)
	lostStage := apptest.DiscoverSeededPipeline(t, e).Lost
	for _, door := range doors {
		items := seedStageDeals(t, e, pipeline, from, 4)
		already := seedStageDeals(t, e, pipeline, to, 1)[0]
		var lost bulkItemDTO
		if status := e.Call(t, "POST", "/v1/deals/"+items[0].ID+"/advance",
			AnyMap{"to_stage_id": lostStage, "lost_reason": "No budget"}, map[string]string{"If-Match": itoa(items[0].Version)}, &lost); status != http.StatusOK {
			t.Fatalf("%s: closing a deal as lost → %d", door.name, status)
		}
		items[0].Version = lost.Version
		if status := e.Call(t, "PATCH", "/v1/deals/"+items[1].ID, AnyMap{"name": "Renamed " + ids.NewV7().String()},
			map[string]string{"If-Match": itoa(items[1].Version)}, nil); status != http.StatusOK {
			t.Fatalf("%s: renaming a deal after it was read → %d", door.name, status)
		}
		selection := append([]bulkItemDTO{already}, items...)

		preview, refusal := door.bulkDoor.preview(t, moveDeals(to, selection))
		if refusal != "" || preview.Count != 2 || len(preview.Excluded) != 3 {
			t.Fatalf("%s: preview → %+v, refused %q; want two to move and three left alone", door.name, preview, refusal)
		}
		out, refusal := door.execute(t, moveDeals(to, selection), "")
		if refusal != "" || out.Changed != 2 {
			t.Fatalf("%s: executing → %+v, refused %q", door.name, out, refusal)
		}
		reasons := map[string]string{}
		for _, skip := range out.Skipped {
			reasons[skip.ID] = skip.Reason
		}
		want := map[string]string{already.ID: "no_change", items[0].ID: "refused", items[1].ID: "changed_since_preview"}
		for id, reason := range want {
			if reasons[id] != reason {
				t.Errorf("%s: deal %s was left alone as %q, want %q", door.name, id, reasons[id], reason)
			}
		}
		if got := stageOfDeal(t, e, items[0].ID); got != lostStage {
			t.Errorf("%s: the lost deal was moved to %s", door.name, got)
		}
	}
}

func TestBothDoorsRefuseAClosingStageAndAnythingButADeal(t *testing.T) {
	t.Parallel()
	e, doors := bulkUndoApp(t, "bulk-stage-refusals")
	pipeline, from, _ := openStagePair(t, e)
	stages := apptest.DiscoverSeededPipeline(t, e)
	for _, door := range doors {
		deals := seedStageDeals(t, e, pipeline, from, 1)
		if _, refusal := door.execute(t, moveDeals(stages.Won, deals), ""); !strings.Contains(refusal, "stage_not_open") {
			t.Errorf("%s: moving to a won stage → %q, want stage_not_open", door.name, refusal)
		}
		if _, refusal := door.execute(t, AnyMap{"record_type": "deal", "verb": "move_stage", "items": deals}, ""); !strings.Contains(refusal, "stage_id") {
			t.Errorf("%s: a move with no stage → %q, want it to name stage_id", door.name, refusal)
		}
		contacts := seedBulkContacts(t, e, 1)
		change := AnyMap{"record_type": "contact", "verb": "move_stage", "stage_id": from, "items": contacts}
		if _, refusal := door.execute(t, change, ""); !strings.Contains(refusal, "verb_not_for_record_type") {
			t.Errorf("%s: moving a contact → %q, want verb_not_for_record_type", door.name, refusal)
		}
		change = AnyMap{"record_type": "deal", "verb": "archive", "stage_id": from, "items": deals}
		if _, refusal := door.execute(t, change, ""); !strings.Contains(refusal, "stage_id") {
			t.Errorf("%s: a stage on an archive → %q, want it refused as not allowed", door.name, refusal)
		}
	}
}

func TestBothDoorsSkipADealThatMovedAgainOnUndo(t *testing.T) {
	t.Parallel()
	e, doors := bulkUndoApp(t, "bulk-stage-undo-moved")
	pipeline, from, to := openStagePair(t, e)
	for _, door := range doors {
		items := seedStageDeals(t, e, pipeline, from, 2)
		out, refusal := door.execute(t, moveDeals(to, items), "")
		if refusal != "" || out.Changed != 2 {
			t.Fatalf("%s: moving two deals → %+v, refused %q", door.name, out, refusal)
		}
		var current struct {
			Version int64 `json:"version"`
		}
		if status := e.Call(t, "GET", "/v1/deals/"+items[0].ID, nil, nil, &current); status != http.StatusOK {
			t.Fatalf("%s: reading the deal → %d", door.name, status)
		}
		if status := e.Call(t, "PATCH", "/v1/deals/"+items[0].ID, AnyMap{"name": "Edited " + ids.NewV7().String()},
			map[string]string{"If-Match": itoa(current.Version)}, nil); status != http.StatusOK {
			t.Fatalf("%s: editing the deal after the batch → %d", door.name, status)
		}
		undone, refusal := door.undo(t, out.BatchID, "")
		if refusal != "" || undone.Changed != 1 || len(undone.Skipped) != 1 ||
			undone.Skipped[0].ID != items[0].ID || undone.Skipped[0].Reason != "changed_since_batch" {
			t.Fatalf("%s: undo → %+v, refused %q; want one back and the edited deal left as changed_since_batch", door.name, undone, refusal)
		}
		if got := stageOfDeal(t, e, items[0].ID); got != to {
			t.Errorf("%s: the edited deal was moved back to %s", door.name, got)
		}
		if got := stageOfDeal(t, e, items[1].ID); got != from {
			t.Errorf("%s: the untouched deal stands on %s, want %s", door.name, got, from)
		}
	}
}

func TestBothDoorsRefuseAStageMoveWhoseTokenWasIssuedForAnotherStage(t *testing.T) {
	t.Parallel()
	e, doors := bulkUndoApp(t, "bulk-stage-token")
	pipeline, from, to := openStagePair(t, e)
	for _, door := range doors {
		items := seedStageDeals(t, e, pipeline, from, 2)
		preview, refusal := door.bulkDoor.preview(t, moveDeals(to, items))
		if refusal != "" || preview.ConfirmToken == "" {
			t.Fatalf("%s: preview → %+v, refused %q", door.name, preview, refusal)
		}
		other := moveDeals(from, items)
		other["confirm_token"] = preview.ConfirmToken
		if _, refusal := door.execute(t, other, ""); !strings.Contains(refusal, "confirm_token") {
			t.Errorf("%s: executing another stage under the token → %q, want a refusal naming confirm_token", door.name, refusal)
		}
		for _, item := range items {
			if got := stageOfDeal(t, e, item.ID); got != from {
				t.Errorf("%s: deal %s moved to %s under a token for another stage", door.name, item.ID, got)
			}
		}
	}
}

func TestBothDoorsCloseNothingOnUndoWhenTheUndoTargetWasRetypedAsWon(t *testing.T) {
	t.Parallel()
	e, doors := bulkUndoApp(t, "bulk-stage-undo-retyped")
	pipeline, from, to := openStagePair(t, e)
	for _, door := range doors {
		items := seedStageDeals(t, e, pipeline, from, 2)
		out, refusal := door.execute(t, moveDeals(to, items), "")
		if refusal != "" || out.Changed != 2 {
			t.Fatalf("%s: moving two deals → %+v, refused %q", door.name, out, refusal)
		}
		// An administrator retypes the emptied stage as won. The edit is written
		// directly, standing in for the stage settings screen.
		if _, err := e.Owner.Exec(t.Context(), `UPDATE stage SET semantic = 'won', win_probability = 100 WHERE id = $1`, from); err != nil {
			t.Fatalf("%s: retyping the stage: %v", door.name, err)
		}
		undone, refusal := door.undo(t, out.BatchID, "")
		if _, err := e.Owner.Exec(t.Context(), `UPDATE stage SET semantic = 'open', win_probability = 0 WHERE id = $1`, from); err != nil {
			t.Fatalf("%s: restoring the stage: %v", door.name, err)
		}
		if refusal != "" || undone.Changed != 0 || len(undone.Skipped) != 2 || undone.Skipped[0].Reason != "refused" ||
			undone.Skipped[0].Code != "stage_not_open" || undone.Skipped[1].Code != "stage_not_open" {
			t.Fatalf("%s: undo → %+v, refused %q; want both deals refused as stage_not_open, the writer's own check, not as a win without evidence", door.name, undone, refusal)
		}
		for _, item := range items {
			if got := stageOfDeal(t, e, item.ID); got != to {
				t.Errorf("%s: deal %s stands on %s after the undo, want it left on %s", door.name, item.ID, got, to)
			}
			if n := countOwned(t, e, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'open'`, item.ID); n != 1 {
				t.Errorf("%s: deal %s is no longer open", door.name, item.ID)
			}
		}
	}
}

func TestBothDoorsSkipAsChangedSinceBatchADealAHumanMovedBackToTheUndoTarget(t *testing.T) {
	t.Parallel()
	e, doors := bulkUndoApp(t, "bulk-stage-undo-movedback")
	pipeline, from, to := openStagePair(t, e)
	for _, door := range doors {
		items := seedStageDeals(t, e, pipeline, from, 1)
		out, refusal := door.execute(t, moveDeals(to, items), "")
		if refusal != "" || out.Changed != 1 {
			t.Fatalf("%s: moving a deal → %+v, refused %q", door.name, out, refusal)
		}
		var current struct {
			Version int64 `json:"version"`
		}
		if status := e.Call(t, "GET", "/v1/deals/"+items[0].ID, nil, nil, &current); status != http.StatusOK {
			t.Fatalf("%s: reading the deal → %d", door.name, status)
		}
		if status := e.Call(t, "POST", "/v1/deals/"+items[0].ID+"/advance", AnyMap{"to_stage_id": from},
			map[string]string{"If-Match": itoa(current.Version)}, nil); status != http.StatusOK {
			t.Fatalf("%s: a human moving the deal back → %d", door.name, status)
		}
		undone, refusal := door.undo(t, out.BatchID, "")
		if refusal != "" || undone.Changed != 0 || len(undone.Skipped) != 1 ||
			undone.Skipped[0].ID != items[0].ID || undone.Skipped[0].Reason != "changed_since_batch" {
			t.Fatalf("%s: undo → %+v, refused %q; want the deal skipped as changed_since_batch", door.name, undone, refusal)
		}
	}
}
