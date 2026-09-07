// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H3

package gates

// The lockfile refresh is not held back by the repo-wide release-age floor.
//
// renovate.json's own description said the floor "does NOT apply here". It did:
// a top-level option inherits into every block that does not override it, and a
// description is not an override. So the daily cadence — which the same
// description calls the ONLY mechanism that adopts a fix for a lockfile-only
// transitive advisory — was in fact holding every refresh on the age of
// whichever transitive package had moved most recently.
//
// The age floor that matters is enforced downstream anyway, by pnpm's own
// supply-chain policy at install time: a too-fresh entry fails CI and so blocks
// automerge. Nothing is loosened by this; what changes is that the block does
// what its description says.
//
// Held here because the failure is silent in the worst way: config that reads
// correctly, behaves otherwise, and shows up only as advisories adopted later
// than anyone intended.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTheLockfileRefreshOverridesTheReleaseAgeFloor(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join(repoRoot, "renovate.json"))
	if err != nil {
		t.Fatalf("reading renovate.json: %v", err)
	}
	// Decoded as a map rather than a tagged struct: renovate.json is somebody
	// else's schema and its keys are camelCase, which the tag linter reads as a
	// convention violation in OUR code. The keys are read as data here, which
	// is what they are.
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("renovate.json is not readable as JSON: %v", err)
	}
	repoWideFloor, _ := config["minimumReleaseAge"].(string)
	block, _ := config["lockFileMaintenance"].(map[string]any)
	enabled, _ := block["enabled"].(bool)
	blockFloor, overridden := block["minimumReleaseAge"].(string)

	// The repo-wide floor is the thing being overridden. Without one there is
	// nothing to inherit and this gate is checking an override of nothing.
	if repoWideFloor == "" {
		t.Fatal("renovate.json declares no repo-wide minimumReleaseAge, so this gate is holding an " +
			"override against a floor that no longer exists — decide whether the block still needs one")
	}
	if !enabled {
		t.Fatal("lockFileMaintenance is not enabled, and it is the only mechanism that adopts a fix " +
			"for a lockfile-only transitive advisory")
	}
	if !overridden {
		t.Fatalf("lockFileMaintenance does not override minimumReleaseAge, so it inherits the "+
			"repo-wide %q. A description saying the floor does not apply is not an override: every "+
			"refresh is then held on the age of whichever transitive package moved most recently, "+
			"and the daily cadence this block exists for becomes weekly in practice",
			repoWideFloor)
	}
	if blockFloor != "0" {
		t.Errorf("lockFileMaintenance sets minimumReleaseAge to %q, which is still a floor — the "+
			"age check that matters here runs downstream, in pnpm's supply-chain policy at install "+
			"time, which fails CI on a too-fresh entry and blocks automerge", blockFloor)
	}
}
