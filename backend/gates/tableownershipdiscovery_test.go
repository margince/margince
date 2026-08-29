// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// WHICH packages the ownership gate walks, derived rather than remembered.
//
// Separate from the judgement next door because it answers a different
// question. That file asks whether a package writes only what it owns; this one
// asks who the writers ARE — and the answer used to be a list somebody had to
// keep, which is the shape a derivation replaces.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// platformRoot is the tree the platform writers are DISCOVERED in. Everything
// under it that writes a row is walked; everything else is not swept in.
const platformRoot = "internal/platform"

// platformStoreDirs answers every package under internal/platform that writes a
// row.
//
// Derived, not named. Three were named here — settings, extsecrets, keyvault —
// and the list was demonstrably short: platform/events writes event_outbox and
// platform/jobs deletes river_job, so neither write was ever compared against
// the ownership map it is declared in. A hand-kept list of writers is one
// somebody has to remember to extend, and nothing fails when they do not; that
// is exactly the hole `vault_secret` sat in until somebody noticed.
//
// Widening to the whole of internal/platform would be the other mistake: it
// would sweep in packages that write no rows and make the gate judge files it
// has no declaration for. Discovery keeps the set exactly as wide as the
// writing.
func platformStoreDirs(t *testing.T) []string {
	t.Helper()
	found := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(platformRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") ||
			isIntegrationTagged(path) {
			return err
		}
		path = filepath.ToSlash(path)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == storekitOwned {
			// The applier itself, not an owner. Its writes name whatever table
			// the CALLER passes, so judging it as a writer would attribute
			// every module's versioned write to this one package — and every
			// one of those is already judged where the table is named.
			return nil
		}
		if writesARow(file) {
			found[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s for row writers: %v", platformRoot, err)
	}
	dirs := make([]string, 0, len(found))
	for dir := range found {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	if len(dirs) < platformWriterFloor {
		t.Fatalf("only %d package(s) under %s were found to write rows, and this gate is pinned "+
			"at %d: %v\n\nA discovery that stopped finding writers reports a clean tree in the "+
			"same words as a tree whose writers are all declared.",
			len(dirs), platformRoot, platformWriterFloor, dirs)
	}
	return dirs
}

// platformWriterFloor is how many row-writing platform packages the discovery
// found when it replaced the hand-kept list.
const platformWriterFloor = 5

// writesARow reports whether the file holds a SQL statement that writes one.
func writesARow(file *ast.File) bool {
	found := false
	for _, statement := range gatekit.SQLStatementsOf(file) {
		if len(sqlWriteTargets(statement)) > 0 {
			found = true
		}
	}
	return found
}

// TestThePlatformWritersAreDiscoveredRatherThanRemembered is the derivation,
// from the other end.
//
// Three platform packages were named here by hand, and the list was short by
// two: platform/events writes event_outbox and platform/jobs deletes river_job,
// so neither write was ever compared against the ownership map it is declared
// in. A hand-kept list of writers is one somebody has to remember to extend,
// and nothing failed when they did not.
func TestThePlatformWritersAreDiscoveredRatherThanRemembered(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	for _, dir := range platformStoreDirs(t) {
		found[dir] = true
	}
	// The three that were named, and the two that were not.
	for _, dir := range []string{
		settingsStoreDir, extSecretsStoreDir, keyVaultStoreDir,
		"internal/platform/events", jobsStoreDir,
	} {
		if !found[dir] {
			t.Errorf("%s writes rows and the discovery did not find it", dir)
		}
	}
	// And the applier is NOT one: its writes name whatever table the caller
	// passes, so counting it would attribute every module's versioned write to
	// one package.
	if found[storekitOwned] {
		t.Errorf("%s was discovered as a writer; it is the mechanism every writer goes through",
			storekitOwned)
	}
	// A package that writes no rows stays outside. Widening to the whole of
	// internal/platform would make the gate judge files it has no declaration
	// for, which is the other way to get this wrong.
	if found["internal/platform/blobstore"] {
		t.Error("internal/platform/blobstore writes no rows and was swept in anyway")
	}
}
