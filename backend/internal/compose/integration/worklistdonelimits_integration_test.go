// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Done refusals name the input to fix, so callers can repair the request.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAWorklistDoneNamesTheInvalidInput(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Done Refusals", "admin@donerefusals.test", "Admin")

	const aRowID = "00000000-0000-7000-8000-0000000000aa"
	cases := map[string]struct {
		body  AnyMap
		field string
	}{
		"no rows":             {AnyMap{"rows": []AnyMap{}}, "rows"},
		"a lane with no Done": {AnyMap{"rows": []AnyMap{{"source": "task", "row_id": aRowID}}}, "rows[0].source"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var problem problemBody
			status := e.Call(t, "PUT", "/v1/worklist/done", tc.body, nil, &problem)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("→ %d %+v, want 422", status, problem)
			}
			if !namesField(problem, tc.field) {
				t.Fatalf("the 422 names %+v, want %s", problem.Details.Errors, tc.field)
			}
		})
	}
}
