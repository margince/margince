// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// overlongCell is longer than any column the importer writes may hold.
var overlongCell = strings.Repeat("a", 5000)

// identityCells are the cells that make a row of each type a valid record of
// its own, keyed by target.
func identityCells(object string, n int) map[string]string {
	switch object {
	case "company":
		return map[string]string{"display_name": fmt.Sprintf("Limits %d GmbH", n)}
	default:
		return map[string]string{"email": fmt.Sprintf("limits%d@limits.example", n), "full_name": fmt.Sprintf("Limit Contact %d", n)}
	}
}

// overlongFile is one row per target the profile offers, each valid but for
// that one target's cell, which is overlong.
func overlongFile(t *testing.T, object string, targets []string) string {
	t.Helper()
	var b strings.Builder
	out := csv.NewWriter(&b)
	if err := out.Write(targets); err != nil {
		t.Fatalf("writing the header: %v", err)
	}
	for n, target := range targets {
		cells := identityCells(object, n)
		cells["ref"] = fmt.Sprintf("R-%d", n)
		cells[target] = overlongCell
		row := make([]string, len(targets))
		for i, column := range targets {
			row[i] = cells[column]
		}
		if err := out.Write(row); err != nil {
			t.Fatalf("writing row %d: %v", n, err)
		}
	}
	out.Flush()
	return b.String()
}

// A cell the store would refuse is a problem the test run reports, never a
// commit that stops part-way. The census is every target the profile offers,
// so a target added later is held to the same rule.
func TestAnOverlongCellInAnyTargetIsReportedByTheTestRunAndNotTheCommit(t *testing.T) {
	for _, object := range []string{"company", "contact", "lead"} {
		t.Run(object, func(t *testing.T) {
			e := setupImportApp(t)
			probe, status := uploadCSV(t, e, object, "ref\nR-0\n")
			if status != http.StatusOK || len(probe.Targets) == 0 {
				t.Fatalf("profiling → %d with targets %v", status, probe.Targets)
			}
			profile, _ := uploadCSV(t, e, object, overlongFile(t, object, probe.Targets))
			mapping := map[string]string{}
			for _, target := range probe.Targets {
				mapping[target] = target
			}
			var run importRunDTO
			if status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
				"connector": "csv", "object": object, "source_ref": profile.SourceRef,
				"mapping": mapping, "source_key": "ref",
			}, nil, &run); status != http.StatusAccepted {
				t.Fatalf("test run → %d", status)
			}
			predicted := readImportReport(t, e, run.ID).Disposition

			e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil)
			if after := readRun(t, e, run.ID); after.Status != "complete" {
				t.Fatalf("the commit ended %q (%s); every cell it refuses must be a problem of the test run", after.Status, after.Error)
			}
			done := readImportReport(t, e, run.ID).Disposition
			if done.Created != predicted.Created || done.Skipped != predicted.Skipped {
				t.Fatalf("committed %+v, the test run said %+v", done, predicted)
			}
		})
	}
}
