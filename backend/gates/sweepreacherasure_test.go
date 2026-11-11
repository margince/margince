// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/corereach"
	"github.com/margince/margince/backend/internal/platform/sweepreach"
)

// declaredErasureStatements are the statements the Art. 17 cascade runs for
// the tables core modules declared (sweepreach), rendered as Erase builds them.
// They count only while the cascade still calls Erase, so dropping that call
// leaves those tables uncovered here.
func declaredErasureStatements(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile("internal/modules/privacy/erasure_declared.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "sweepreach.Erase(") {
		return nil
	}
	var out []string
	for _, o := range corereach.Tables() {
		if len(o.Table.Erase) > 0 {
			out = append(out, strings.ReplaceAll(sweepreach.EraseSQL(o), `"`, ""))
		}
	}
	return out
}
