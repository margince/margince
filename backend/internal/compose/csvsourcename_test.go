// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheSuggestedSourceNameIsOneTheStageAccepts(t *testing.T) {
	for _, file := range []string{"HubSpot export (2).zip", "pipedrive-export.zip", "Ümlaut/Daten.zip", strings.Repeat("x", 120) + ".zip"} {
		name := suggestedSourceName(file)
		if !sourceNamePattern.MatchString(name) || len(name) > 80 {
			t.Errorf("%q → %q, which the stage refuses with a 422", file, name)
		}
	}
	if got := suggestedSourceName("pipedrive-export.zip"); got != "pipedrive-export" {
		t.Errorf("got %q, want the stem", got)
	}
}

func TestAnEmptySourceNameKeepsTheNamespaceEarlierRunsUsed(t *testing.T) {
	if got := importSourceSystem(""); got != provenance.ReservedSourceSystemPrefix+migration.ConnectorCSV {
		t.Fatalf("got %q: a single CSV re-uploaded after this change would stop finding what it landed", got)
	}
}

func TestTwoSourceNamesAreTwoNamespaces(t *testing.T) {
	if importSourceSystem("hubspot") == importSourceSystem("pipedrive") {
		t.Fatal("ref 1 from two old CRMs would land on one record")
	}
}

// The read wire names a CSV run by its source name. That holds only while the
// provenance package and the importer spell the connector segment alike.
func TestAnImportedRowReadsAsItsSourceName(t *testing.T) {
	stored := importSourceSystem("bundle")
	if got := provenance.ImportedFrom(&stored); got == nil || *got != "bundle" {
		t.Fatalf("ImportedFrom(%q) = %v, want bundle", stored, got)
	}
}
