// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// No internal requirement code reaches the copy a model reads.
//
// A requirement code names a line in a planning record no reader of the
// catalog can open. To a model it is noise that costs tokens on every
// tools/list. The corpus is what is served: the published MCP
// catalog, and the REST skill's contract and index, each built from the
// contract. A code stays welcome in a Go comment or a contract comment.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// restSkillFiles are the REST skill's served contract and index.
var restSkillFiles = []string{
	"internal/compose/agentbundle/agentcontract_gen.yaml",
	"internal/compose/agentbundle/agentindex_gen.txt",
}

// requirementCode matches the shapes the planning records use. One is a
// capital A and two or three digits. The others are AC-<area>-<n>, B-E<n>.<n>,
// and capitalised words joined by hyphens that end in a number.
var requirementCode = regexp.MustCompile(
	`\b(?:A\d{2,3}|AC-[a-z]+-\d+|[BS]-EP?\d+(?:\.\d+[a-z]?)*|[A-Z]{2,}(?:-[A-Z]+)*-\d+[a-z]?)\b`)

// standardNames are published names of the same shape, such as ISO-8601. ADR
// is a decision label, which the rulebook lets copy show.
var standardNames = []string{"ISO-", "UTF-", "SHA-", "BUSL-", "RFC-", "ADR-"}

func TestNoServedCopyCarriesARequirementCode(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(publishedCatalog)
	if err != nil {
		t.Fatalf("reading the published catalog: %v", err)
	}
	var catalog any
	if err := json.Unmarshal(source, &catalog); err != nil {
		t.Fatalf("parsing the published catalog: %v", err)
	}
	said := catalogStrings(catalog)
	if len(said) < catalogStringFloor {
		t.Fatalf("read %d string(s) out of %s, expected at least %d", len(said), publishedCatalog, catalogStringFloor)
	}
	corpus := map[string][]string{publishedCatalog: sortedCatalogKeys(said)}
	for _, path := range restSkillFiles {
		text, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if len(text) == 0 {
			t.Fatalf("%s is empty, so this would check nothing", path)
		}
		corpus[path] = strings.Split(string(text), "\n")
	}
	for path, lines := range corpus {
		for _, line := range lines {
			for _, code := range requirementCode.FindAllString(line, -1) {
				if !isStandardName(code) {
					t.Errorf("%s serves the requirement code %s in %q. Take it out of the summary or description "+
						"in backend/api/crm.yaml or the tool's Go copy; a comment may keep it", path, code, trimForFinding(line))
				}
			}
		}
	}
}

func isStandardName(code string) bool {
	for _, prefix := range standardNames {
		if strings.HasPrefix(code, prefix) {
			return true
		}
	}
	return false
}
