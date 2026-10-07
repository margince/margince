// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A merge cannot be undone, so If-Match is the one way a caller says "merge the
// record as I read it". A stale version has to be refused with version_skew and
// leave both records alone; the current version, or no header, goes through.
func TestAMergeHonoursTheSourcesIfMatchVersion(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Merge If-Match", "merge-ifmatch@test.test", "Admin")

	for _, kind := range []struct{ path, nameField, source string }{
		{"/v1/contacts", "full_name", "manual"},
		{"/v1/companies", "display_name", "manual"},
	} {
		t.Run(kind.path, func(t *testing.T) {
			create := func(name string) string {
				return createAndID(t, e, kind.path, AnyMap{kind.nameField: name, "source": kind.source})
			}
			version := func(id string) int64 {
				var row struct {
					Version int64 `json:"version"`
				}
				if status := e.Call(t, "GET", kind.path+"/"+id, nil, nil, &row); status != http.StatusOK {
					t.Fatalf("read %s → %d", id, status)
				}
				return row.Version
			}
			merge := func(source, target, ifMatch string) (int, problemBody) {
				headers := map[string]string{}
				if ifMatch != "" {
					headers["If-Match"] = ifMatch
				}
				var problem problemBody
				status := e.Call(t, "POST", kind.path+"/"+source+"/merge", AnyMap{"target_id": target}, headers, &problem)
				return status, problem
			}
			archived := func(id string) bool {
				var row struct {
					ArchivedAt *string `json:"archived_at"`
				}
				if status := e.Call(t, "GET", kind.path+"/"+id, nil, nil, &row); status != http.StatusOK {
					t.Fatalf("read %s → %d", id, status)
				}
				return row.ArchivedAt != nil
			}

			target := create("Survivor " + kind.nameField)
			source := create("Source " + kind.nameField)
			current := version(source)

			// 409 alone is not the claim: a merge answers it for an already-merged
			// source and an unwritable target too.
			if status, problem := merge(source, target, strconv.FormatInt(current+98, 10)); status != http.StatusConflict || problem.Code != "version_skew" {
				t.Errorf("merge with a version the source is not at answered %d %q, want 409 version_skew", status, problem.Code)
			}
			if archived(source) {
				t.Fatal("a refused merge still archived the source")
			}

			if status, _ := merge(source, target, strconv.FormatInt(current, 10)); status != http.StatusOK {
				t.Fatalf("merge at the source's current version answered %d, want 200", status)
			}
			if !archived(source) {
				t.Error("the merge at the current version did not archive the source")
			}

			// No header is no precondition.
			bare := create("Bare " + kind.nameField)
			if status, _ := merge(bare, target, ""); status != http.StatusOK {
				t.Errorf("merge with no If-Match answered %d, want 200", status)
			}
		})
	}
}
