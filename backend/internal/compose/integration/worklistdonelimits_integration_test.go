// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Worklist Done whose body breaks the limits the contract declares is the
// contract's 422 naming `rows`, not a bare 400.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAWorklistDoneBreakingItsDeclaredLimitsIsA422NamingRows(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Done Refusals", "admin@donerefusals.test", "Admin")

	const aRowID = "00000000-0000-7000-8000-0000000000aa"
	cases := map[string]AnyMap{
		"no rows":             {"rows": []AnyMap{}},
		"a lane with no Done": {"rows": []AnyMap{{"source": "task", "row_id": aRowID}}},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var problem problemBody
			status := e.Call(t, "PUT", "/v1/worklist/done", body, nil, &problem)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("→ %d %+v, want 422", status, problem)
			}
			if !namesField(problem, "rows") {
				t.Fatalf("the 422 names %+v, want rows", problem.Details.Errors)
			}
		})
	}
}
