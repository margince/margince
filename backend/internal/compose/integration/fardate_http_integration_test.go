// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// fieldRefusal is the part of a 422 problem body these tests read: which
// field was refused, and for what.
type fieldRefusal struct {
	Code    string `json:"code"`
	Details struct {
		Errors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	} `json:"details"`
}

func (f fieldRefusal) names(field string) bool {
	for _, refused := range f.Details.Errors {
		if refused.Field == field && refused.Code == "out_of_range" {
			return true
		}
	}
	return false
}

// A date-time that some zone renders as year 10000 cannot be encoded, so a
// writer that stored one would blank every list holding its row. The bound
// refuses it at the door, and the last instant inside the bound still lists.
func TestADateTimeNoZoneCanRenderIsRefusedAndTheListsStayReadable(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Far Dates", "admin@fardates.test", "Admin")

	writers := []struct {
		name, path, field, list string
		body                    func(at string) AnyMap
	}{
		{
			name: "task due_at", path: "/v1/tasks", field: "due_at", list: "/v1/activities?kind=task",
			body: func(at string) AnyMap { return AnyMap{"subject": "far task", "due_at": at} },
		},
		{
			name: "signal detected_at", path: "/v1/signals", field: "detected_at", list: "/v1/signals",
			body: func(at string) AnyMap {
				return AnyMap{"kind": "risk", "summary": "far signal", "source": "manual", "detected_at": at}
			},
		},
	}
	for _, writer := range writers {
		t.Run(writer.name, func(t *testing.T) {
			for _, refused := range []string{"9999-12-31T23:59:59Z", "9999-12-31T00:00:00Z", "0001-01-01T12:00:00Z", "0000-06-01T00:00:00Z"} {
				var problem fieldRefusal
				status := e.Call(t, http.MethodPost, writer.path, writer.body(refused), nil, &problem)
				if status != http.StatusUnprocessableEntity || !problem.names(writer.field) {
					t.Fatalf("%s %s → %d %+v, want 422 out_of_range on %s", writer.field, refused, status, problem, writer.field)
				}
			}

			if status := e.Call(t, http.MethodPost, writer.path, writer.body("9999-12-30T23:59:59Z"), nil, nil); status != http.StatusCreated {
				t.Fatalf("%s at the last accepted instant → %d, want 201", writer.field, status)
			}
			var page struct {
				Data []map[string]any `json:"data"`
			}
			if status := e.Call(t, http.MethodGet, writer.list, nil, nil, &page); status != http.StatusOK || len(page.Data) == 0 {
				t.Fatalf("GET %s → %d with %d rows, want 200 listing the row just written", writer.list, status, len(page.Data))
			}
		})
	}
}
