// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every writer of a stage move records the stage order it happened in. It also
// records whether the move was made in Margince or an import brought it in. Every writer of a lead's
// arrival records the owner it arrived with. Writers are found by the statement
// they send, so a new one is judged without a list. The cohort readings mark a
// missing value as partial, so a skip degrades silently.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	stageMoveInsert   = regexp.MustCompile(`(?is)INSERT\s+INTO\s+deal_stage_history\s*\(([^)]*)\)`)
	leadInsert        = regexp.MustCompile(`(?is)INSERT\s+INTO\s+lead\s*\(([^)]*)\)`)
	leadUpdate        = regexp.MustCompile(`(?is)UPDATE\s+lead\s+SET\s+(.*?)\bWHERE\b`)
	routedAtAssigned  = regexp.MustCompile(`(?i)\brouted_at\s*=`)
	routedAtPatched   = regexp.MustCompile(`\.Set\(\s*"routed_at"`)
	arrivalStampOwner = "internal/modules/contacts/leadresponseclock.go"
)

// productSources are the hand-written, non-test Go files of the backend tree,
// keyed by path.
func productSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == "testdata" || path == filepath.Join(root, "internal", "contracts") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEveryStageMoveWriterRecordsTheStageOrderAtTheMove(t *testing.T) {
	t.Parallel()
	writers := 0
	for path, body := range productSources(t) {
		for _, columns := range stageMoveInsert.FindAllStringSubmatch(body, -1) {
			writers++
			for _, want := range []string{"from_position_at_change", "to_position_at_change"} {
				if !strings.Contains(columns[1], want) {
					t.Errorf("%s inserts a stage move without %s: a later reorder would then decide which way it went", path, want)
				}
			}
			if !slices.Contains(strings.Fields(strings.ReplaceAll(columns[1], ",", " ")), "provenance") {
				t.Errorf("%s inserts a stage move without provenance: stage conversion would read it as unknown history", path)
			}
		}
	}
	if writers == 0 {
		t.Fatal("found no INSERT INTO deal_stage_history in the backend tree: the census is short, so it proves nothing")
	}
}

func TestEveryLeadArrivalWriterRecordsTheOwnerItArrivedWith(t *testing.T) {
	t.Parallel()
	inserts, stamps := 0, 0
	for path, body := range productSources(t) {
		for _, columns := range leadInsert.FindAllStringSubmatch(body, -1) {
			inserts++
			if !strings.Contains(columns[1], "arrival_owner_id") && !strings.Contains(columns[1], "arrivalOwnerColumn") {
				t.Errorf("%s inserts a lead without arrival_owner_id: its arrival would have no owner", path)
			}
		}
		for _, set := range leadUpdate.FindAllStringSubmatch(body, -1) {
			if routedAtAssigned.MatchString(set[1]) {
				stamps++
				if !strings.Contains(set[1], "arrival_owner_id") && !strings.Contains(set[1], "arrivalOwnerColumn") {
					t.Errorf("%s stamps routed_at without arrival_owner_id: the lead's arrival would keep no owner", path)
				}
			}
		}
		if !strings.HasSuffix(path, arrivalStampOwner) && routedAtPatched.MatchString(body) {
			t.Errorf("%s patches routed_at itself; call stampArrival in %s, which records the arrival owner beside it", path, arrivalStampOwner)
		}
		if strings.HasSuffix(path, arrivalStampOwner) {
			stamps += len(routedAtPatched.FindAllString(body, -1))
		}
	}
	if inserts == 0 || stamps == 0 {
		t.Fatalf("found %d lead inserts and %d routed_at stamps in the backend tree: a short census proves nothing", inserts, stamps)
	}
}
