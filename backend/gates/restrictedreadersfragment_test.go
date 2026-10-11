// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A fragment that reaches its query through another constant is still judged
// at the function that runs it. The planted package names the activity
// fragment directly only in a guarded function, and runs it unguarded through
// a second constant. Following direct names alone reads that as clean.
func TestARestrictedReaderCensusFollowsAFragmentThroughAConstant(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	planted := filepath.Join(dir, "planted.go")
	source := "package planted\n\n" +
		"const sourceProbe = `EXISTS (SELECT 1 FROM activity a WHERE a.id = ev.source_id)`\n\n" +
		"const columns = `id, ` + sourceProbe\n\n" +
		"func guarded(q func(string)) { q(`SELECT ` + sourceProbe + ` AND a.restricted_at IS NULL`) }\n\n" +
		"func unguarded(q func(string)) { q(`SELECT ` + columns + ` FROM deal_stage_evidence ev`) }\n"
	if err := os.WriteFile(planted, []byte(source), 0o600); err != nil {
		t.Fatalf("planting the package: %v", err)
	}
	file, err := gatekit.ParseFile(planted, 0)
	if err != nil {
		t.Fatalf("parsing the planted package: %v", err)
	}
	offenders := unguardedActivityReaders(packageCallGraph(t, dir),
		packageBindingNames(parsePackageFiles(t, dir)), file, heldDimension)
	if !slices.ContainsFunc(offenders, func(o string) bool { return strings.HasPrefix(o, "unguarded:") }) {
		t.Fatalf("offenders = %v, want the function that runs the fragment through columns", offenders)
	}
	if slices.ContainsFunc(offenders, func(o string) bool { return strings.HasPrefix(o, "guarded:") }) {
		t.Errorf("offenders = %v, the guarded namer is reported too", offenders)
	}
}
