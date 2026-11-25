// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The absent preference is two halves of one promise in the contract: an
// operation that takes `Prefer` (PreferAbsent) may answer the 204 NothingYet
// response, and only such an operation does. The server serves the preference
// on exactly the operations that declare the header, so a 204 declared without
// the header is an answer no request can get, and a header without the 204 is
// an answer no client was told about.

import (
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	preferAbsentRef = "#/components/parameters/PreferAbsent"
	nothingYetRef   = "#/components/responses/NothingYet"
)

type absentOperation struct {
	Parameters []struct {
		Ref string `yaml:"$ref"`
	} `yaml:"parameters"`
	Responses map[string]struct {
		Ref string `yaml:"$ref"`
	} `yaml:"responses"`
}

func TestThePreferHeaderAndThe204AreDeclaredTogether(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	declared := 0
	var mismatched []string
	for path, item := range doc.Paths {
		for method, node := range item {
			if method == "parameters" || method == "summary" || method == "description" {
				continue
			}
			var op absentOperation
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			takesPrefer := false
			for _, p := range op.Parameters {
				takesPrefer = takesPrefer || p.Ref == preferAbsentRef
			}
			answers204 := op.Responses["204"].Ref == nothingYetRef
			if takesPrefer {
				declared++
			}
			if takesPrefer != answers204 || (takesPrefer && method != "get") {
				mismatched = append(mismatched, method+" "+path)
			}
		}
	}
	sort.Strings(mismatched)
	for _, op := range mismatched {
		t.Errorf("%s: a read takes PreferAbsent exactly when it declares '204': {$ref: %s}", op, nothingYetRef)
	}
	if declared == 0 {
		t.Fatal("no operation takes PreferAbsent: the census read nothing, or the parameter was renamed")
	}
}
