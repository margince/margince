// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The fleet enumeration lives at ratified sites only. A `SELECT id FROM
// workspace` inside a module is the signature of the anti-pattern this phase
// removed: one job row looping every tenant, logging each failure and
// returning success, so River records a green sweep over workspaces that
// failed. Dispatchers enumerate; workers take the workspace they were given.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// fleetScanMarker is any read of the workspace COLLECTION. It is deliberately
// just `FROM workspace`, not the full `WHERE archived_at IS NULL` predicate:
// idempotency retention enumerates every workspace INCLUDING archived ones, so
// a marker built around the archived-at filter would never have seen the one
// fleet loop that does not use it — the gate would have been green while the
// anti-pattern lived on.
//
// It still only catches the literal spelling. An alias, a newline inside the
// FROM clause, a schema qualification, or a view would evade it. That residual
// is accepted because the gate's job is to stop the shape being re-introduced
// by habit, not by determined circumvention.
const fleetScanMarker = "FROM workspace"

// singleRowPredicates mark a read of ONE workspace — `WHERE id = $1` and its
// siblings. Those are not fleet enumerations and vastly outnumber them, so the
// gate would be useless without this exclusion.
//
// Matched as a PREFIX rather than requiring a bind parameter: the tree also
// resolves one workspace by correlated subquery (`WHERE id = d.workspace_id`)
// and through the GUC (`WHERE id = NULLIF(current_setting(...))`). Any
// predicate equating id to a single value is a single-row read by
// construction — with the exception of a SET-valued RHS, which setPredicates
// takes back. Checked on the marker's own line and the one after it, because
// the repo splits some query literals across lines.
var singleRowPredicates = []string{"WHERE id = ", "WHERE id=", "WHERE w.id = "}

// setPredicates are the RHS spellings that equate id to a SET rather than to
// one value. They look like a single-row predicate to the prefixes above and
// are not one: `WHERE id = ANY($1::uuid[])` reads many workspaces.
var setPredicates = []string{"= ANY", "= any", "IN (", "in ("}

// ratifiedFleetScan is one sanctioned enumeration site. The COUNT is
// load-bearing: several ratified files hold a legitimate scan alongside code
// that must never grow another, and a bare path waiver would wave through a
// re-introduced sweep loop in the same file.
type ratifiedFleetScan struct {
	count  int
	reason string
}

// ratifiedFleetScans are the sanctioned enumeration sites, keyed by
// repo-relative path. A site missing here is a finding; an entry matching no
// remaining site is stale; a count that no longer matches is either a new scan
// to justify or a removed one to un-ratify.
//
// A reason must name which of exactly four things the site is:
//
//  1. a DISPATCHER's own enumeration — it reads the fleet only to enqueue one
//     workspace-scoped job per tenant, and does no tenant work itself;
//  2. a READ — the enumeration and everything it drives write nothing;
//  3. a BOOT path — it runs once at startup, before any job exists to run it;
//  4. TENANT RESOLUTION for an untenanted inbound request — a push or a webhook
//     carries no tenant, so the scan finds the ONE workspace that owns it and
//     the work then happens in that workspace alone.
//
// What a reason may NOT rest on is the site being outside the job layer. Every
// fleet pass is a job now, so "not reachable from a Work method" describes
// nothing durable: the next pass to want this enumeration reaches it from a
// worker, and a waiver granted on that basis would already have waved it
// through.
var ratifiedFleetScans = map[string]ratifiedFleetScan{
	"internal/platform/database/workspaces.go": {
		2,
		"the scans of the workspace table: LiveWorkspace is what every pass over the installation, due-scan and tenant resolution starts from, and EveryWorkspace is what the privacy cleanups start from, because archiving a workspace does not un-store its data. Neither does tenant work itself",
	},
	"internal/modules/identity/installation.go": {
		1,
		"boot path: resolves the singleton company and refuses to serve when a second exists (ADR-0061 §3) — it IS the workspace authority, not a consumer of it",
	},
}

// countFleetScans returns how many workspace-COLLECTION reads a file makes.
func countFleetScans(src string) int {
	lines := strings.Split(src, "\n")
	n := 0
	for i, line := range lines {
		for offset := 0; ; {
			at := strings.Index(line[offset:], fleetScanMarker)
			if at < 0 {
				break
			}
			end := offset + at + len(fleetScanMarker)
			offset = end
			// A sibling TABLE — workspace_signing_key, workspace_email_domain —
			// is not the workspace collection, so the marker must not run into
			// an identifier character.
			if end < len(line) {
				if c := line[end]; c == '_' || isAlphanumeric(c) {
					continue
				}
			}
			window := line
			if i+1 < len(lines) {
				window += " " + lines[i+1]
			}
			single := false
			for _, pred := range singleRowPredicates {
				if strings.Contains(window, pred) {
					single = true
					break
				}
			}
			for _, pred := range setPredicates {
				if strings.Contains(window, pred) {
					single = false
					break
				}
			}
			if !single {
				n++
			}
		}
	}
	return n
}

func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func TestFleetEnumerationOnlyAtRatifiedSites(t *testing.T) {
	t.Parallel()
	found := map[string]int{}
	paths, err := goFilesUnder("internal")
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	for _, path := range paths {
		// This gate judges PRODUCTION passes: the anti-pattern is one job row
		// looping every tenant. An integration-tagged file is test harness — it
		// never ships, and it has no job to take a workspace from — so it is out
		// of scope by rule. goFilesUnder's _test.go skip does not cover it: a
		// shared fixture promoted into a non-test file to become importable is
		// still harness.
		//
		// The migrate-once gate scopes the other way and INCLUDES these files, for
		// the same reason read backwards: its obligation is about the integration
		// lane, so a promoted fixture is exactly what it must still see.
		if isIntegrationTagged(path) {
			continue
		}
		src, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if n := countFleetScans(string(src)); n > 0 {
			found[filepath.ToSlash(path)] = n
		}
	}

	for path, n := range found {
		ratified, ok := ratifiedFleetScans[path]
		if !ok {
			t.Errorf("%s enumerates the fleet. A per-workspace pass takes its workspace from job args; only a dispatcher enumerates. If this site really must, ratify it in ratifiedFleetScans with a rationale.", path)
			continue
		}
		if n != ratified.count {
			t.Errorf("%s holds %d fleet scans, ratified for %d. A new one needs its own rationale; a removed one needs the count lowered.", path, n, ratified.count)
		}
	}
	for path, ratified := range ratifiedFleetScans {
		if ratified.reason == "" {
			t.Errorf("%s: waiver without a rationale", path)
		}
		if found[path] == 0 {
			t.Errorf("%s: stale waiver — the site no longer enumerates the fleet; delete it", path)
		}
	}
}

// TestModulesReadTheWorkspaceCollectionOnlyInIdentity has no waiver list.
// A module other than identity, which owns the workspace table, takes the fleet
// from database.LiveWorkspace and never spells the
// scan itself. The census check fails when few module files were read, so a
// moved directory cannot turn this into a pass over nothing.
func TestModulesReadTheWorkspaceCollectionOnlyInIdentity(t *testing.T) {
	t.Parallel()
	paths, err := goFilesUnder("internal/modules")
	if err != nil {
		t.Fatalf("walking internal/modules: %v", err)
	}
	if len(paths) < 100 {
		t.Fatalf("read %d module files, want at least 100: the walk lost the module tree", len(paths))
	}
	for _, path := range paths {
		slashed := filepath.ToSlash(path)
		if strings.HasPrefix(slashed, "internal/modules/identity/") || isIntegrationTagged(path) {
			continue
		}
		src, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if n := countFleetScans(string(src)); n > 0 {
			t.Errorf("%s reads the workspace collection %d time(s); call database.LiveWorkspace or database.EveryWorkspace instead", slashed, n)
		}
	}
}
