// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"slices"
	"testing"
)

// The contract's report keys are what an assistant holding only the REST skill
// can choose from, so they must be the keys prebuiltReports serves: a report
// the engine adds is listed, and one it drops stops being offered. The
// derivation route takes a key from a handle the server sent, so it lists none.
func TestTheContractListsTheReportsTheEngineServes(t *testing.T) {
	const path = "/reports/{report}"
	item := loadContract(t).Paths.Find(path)
	if item == nil {
		t.Fatalf("the contract has no %s path", path)
	}
	param := item.Parameters.GetByInAndName("path", "report")
	if param == nil || param.Schema == nil || param.Schema.Value == nil {
		t.Fatalf("%s declares no report path parameter schema", path)
	}
	listed := make([]string, 0, len(param.Schema.Value.Enum))
	for _, key := range param.Schema.Value.Enum {
		name, ok := key.(string)
		if !ok {
			t.Fatalf("%s lists a report key that is not a string: %v", path, key)
		}
		listed = append(listed, name)
	}
	slices.Sort(listed)
	if served := slices.Sorted(maps.Keys(prebuiltReports)); !slices.Equal(listed, served) {
		t.Errorf("%s lists report keys %v, but the engine serves %v. Make the parameter's enum "+
			"in api/crm.yaml match prebuiltReports and regenerate", path, listed, served)
	}
}
