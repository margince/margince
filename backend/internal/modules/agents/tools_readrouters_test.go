// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// Each target refuses a bad argument itself, which proves the call reached it
// with the view gone and the rest intact.
func TestReadRoutersRouteEveryView(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	RegisterAssuranceTool(r, nil)
	RegisterInputChecksTool(r, nil)
	RegisterCoverageTool(r, nil)
	RegisterImportTools(r, stubImports{})
	for _, tc := range []struct {
		router string
		table  map[string]string
		args   string
	}{
		{"read_import", (readImport{}).routeTargets(), `"run_id":"not-a-uuid"`},
	} {
		for view, target := range tc.table {
			served, err := r.InvokeServed(readerCtx(), tc.router, json.RawMessage(`{"view":"`+view+`",`+tc.args+`}`))
			if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != target {
				t.Errorf("%s %s: served %q with %v, want %s's own refusal", tc.router, view, served.Tool, err, target)
			}
		}
	}
}
