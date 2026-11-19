// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheSuggestedSourceNameIsOneTheStageAccepts(t *testing.T) {
	for _, file := range []string{"HubSpot export (2).zip", "pipedrive-export.zip", "Ümlaut/Daten.zip", strings.Repeat("x", 120) + ".zip"} {
		name := SuggestedSourceName(file)
		if !SourceNamePattern.MatchString(name) || len(name) > 80 {
			t.Errorf("%q → %q, which the stage refuses with a 422", file, name)
		}
	}
	if got := SuggestedSourceName("pipedrive-export.zip"); got != "pipedrive-export" {
		t.Errorf("got %q, want the stem", got)
	}
}

func TestAnEmptySourceNameKeepsTheNamespaceEarlierRunsUsed(t *testing.T) {
	if got := SourceSystem(""); got != provenance.ReservedSourceSystemPrefix+migration.ConnectorCSV {
		t.Fatalf("got %q: a single CSV re-uploaded after this change would stop finding what it landed", got)
	}
}

func TestTwoSourceNamesAreTwoNamespaces(t *testing.T) {
	if SourceSystem("hubspot") == SourceSystem("pipedrive") {
		t.Fatal("ref 1 from two old CRMs would land on one record")
	}
}

// The read wire names a CSV run by its source name. That holds only while the
// provenance package and the importer spell the connector segment alike.
func TestAnImportedRowReadsAsItsSourceName(t *testing.T) {
	stored := SourceSystem("bundle")
	if got := provenance.ImportedFrom(&stored); got == nil || *got != "bundle" {
		t.Fatalf("ImportedFrom(%q) = %v, want bundle", stored, got)
	}
}

func TestTheSourceNameRuleNamesEveryCharacterItAllows(t *testing.T) {
	if name, err := SourceName(new(" Fair 2026_v1.2-final ")); err != nil || name != "Fair 2026_v1.2-final" {
		t.Fatalf("a name of allowed characters = %q, %v, want it trimmed and kept", name, err)
	}
	for _, refused := range []string{"wiz ünïcödé!", strings.Repeat("a", SourceNameMax+1)} {
		_, err := SourceName(&refused)
		fault := oneFieldRefusal(t, err, "source_name", "invalid")
		if fault.Max != SourceNameMax {
			t.Errorf("max = %d, want %d", fault.Max, SourceNameMax)
		}
		for _, words := range []string{"A to Z without accents", "0 to 9", "spaces", "(.)", "(_)", "(-)"} {
			if !strings.Contains(fault.Message, words) {
				t.Errorf("message = %q, want it to name %q", fault.Message, words)
			}
		}
	}
}
