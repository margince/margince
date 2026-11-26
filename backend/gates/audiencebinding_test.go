// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestAnAudienceGuardTravelsOnlyThroughTheBindingAReaderUses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	planted := filepath.Join(dir, "planted.go")
	source := `package planted
 import "example.test/auth"
 var guard = auth.WorkspaceAudienceSQL("a")
 var guardedQuery = "SELECT a.body FROM activity a WHERE " + guard
 func guarded(q func(string)) { q(guardedQuery) }
 func bare(q func(string)) { q("SELECT a.body FROM activity a") }
 func shadowed(q func(string)) { guardedQuery := "SELECT a.body FROM activity a"; q(guardedQuery) }
 `
	if err := os.WriteFile(planted, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := gatekit.ParseFile(planted, 0)
	if err != nil {
		t.Fatal(err)
	}
	offenders := unguardedActivityReaders(packageCallGraph(t, dir), packageBindingNames(parsePackageFiles(t, dir)), file, audienceDimension)
	for name, want := range map[string]bool{"guarded": false, "bare": true, "shadowed": true} {
		got := slices.ContainsFunc(offenders, func(o string) bool { return strings.HasPrefix(o, name+":") })
		if got != want {
			t.Errorf("%s reported=%v, want %v: %v", name, got, want, offenders)
		}
	}
}
