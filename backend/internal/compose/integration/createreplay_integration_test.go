// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The create forms send one Idempotency-Key per attempt. A press the page did
// not hold back then reaches the server as a repeat of the first request.
func TestAFormPressedTwiceMakesOneRecord(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	cases := []struct {
		name string
		path string
		list string
		body AnyMap
	}{
		{"contact", "/v1/contacts", "/v1/contacts?q=Twice+Contact", AnyMap{"source": "manual", "full_name": "Twice Contact"}},
		{"company", "/v1/companies", "/v1/companies?q=Twice+Company", AnyMap{"source": "manual", "display_name": "Twice Company"}},
		{"deal", "/v1/deals", "/v1/deals?q=Twice+Deal", AnyMap{
			"source": "manual", "name": "Twice Deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		}},
		{"task", "/v1/tasks", "/v1/activities?kind=task", AnyMap{"source": "manual", "subject": "Twice Task"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{"Idempotency-Key": "press-twice-" + tc.name}
			const presses = 6
			ids := make([]string, presses)
			statuses := make([]int, presses)
			var wg sync.WaitGroup
			for i := range presses {
				wg.Go(func() {
					var made AnyMap
					statuses[i] = e.Call(t, http.MethodPost, tc.path, tc.body, headers, &made)
					ids[i], _ = made["id"].(string)
				})
			}
			wg.Wait()

			created := ""
			for i, status := range statuses {
				switch status {
				case http.StatusCreated:
					if ids[i] == "" || (created != "" && created != ids[i]) {
						t.Fatalf("press %d answered %d with id %q after %q", i, status, ids[i], created)
					}
					created = ids[i]
				case http.StatusConflict:
					// A press that met the first still in flight is refused, never run.
				default:
					t.Fatalf("press %d status = %d, want 201 or 409", i, status)
				}
			}
			if created == "" {
				t.Fatalf("no press created the record: %v", statuses)
			}

			var listed AnyMap
			if status := e.Call(t, http.MethodGet, tc.list, nil, nil, &listed); status != http.StatusOK {
				t.Fatalf("list = %d %v", status, listed)
			}
			rows, _ := listed["data"].([]any)
			if len(rows) != 1 {
				t.Fatalf("%d records after %d presses with one key, want 1", len(rows), presses)
			}

			var again AnyMap
			if status := e.Call(t, http.MethodPost, tc.path, tc.body, headers, &again); status != http.StatusCreated || again["id"] != created {
				t.Fatalf("replay = %d id %v, want 201 and %q", status, again["id"], created)
			}
		})
	}
}
