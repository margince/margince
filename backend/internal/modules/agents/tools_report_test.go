// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"slices"
	"testing"
)

// The `report` argument is closed to the catalog's keys, so a caller reads the
// answer instead of guessing it — and the schema stays valid JSON either way.
func TestRunReportClosesTheReportArgumentToTheCatalog(t *testing.T) {
	for _, tc := range []struct {
		name     string
		catalog  []ReportCatalogEntry
		wantEnum []string
	}{
		{"a catalog closes the argument", probeReportCatalog, []string{"deals-by-stage"}},
		{"an empty catalog omits the enum rather than emitting an unsatisfiable one", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Decoded into a checked shape rather than walked with bare type
			// assertions: a schema regression should FAIL this test, not panic
			// inside it, and a panic reports the wrong thing about the wrong line.
			var parsed struct {
				Properties struct {
					Report struct {
						Enum []string `json:"enum"`
					} `json:"report"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(runReport{catalog: tc.catalog}.Spec().InputSchema, &parsed); err != nil {
				t.Fatalf("InputSchema is not valid JSON, or `report` is not the shape this asserts: %v", err)
			}
			if !slices.Equal(parsed.Properties.Report.Enum, tc.wantEnum) {
				t.Errorf("report enum = %v, want %v", parsed.Properties.Report.Enum, tc.wantEnum)
			}
		})
	}
}

// The two obligations the recital used to carry are now the document's, and
// each is held there rather than dropped: that an empty vocabulary says so
// (TestAnEmptyVocabularyIsPublishedAsAnEmptyListNotNull) and that a rendered
// pipeline id says where one comes from (TestTheProvenanceNoteIsKeyedOnWhatIsRendered).
// What run_report's own description owes is held by
// TestRunReportNamesTheDocumentWithoutOrderingARead.
