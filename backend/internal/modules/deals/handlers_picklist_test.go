// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
)

func TestADealWriteLeavesEveryCheckButItsPicklistsToTheSharedNet(t *testing.T) {
	t.Parallel()
	faulttest.AssertCheckViolationsReachTheSharedNet(t, dealTable, writeStoreErr)
}

// A deal picklist CHECK is one whose body holds a column to `= ANY (ARRAY[...])`.
var dealPicklistCheck = regexp.MustCompile(`^public\.deal\.([a-z_]+) CHECK .*\(([a-z_]+) = ANY \(ARRAY\[`)

// Each mapped constraint is a picklist CHECK on the deal over the field it
// names. Each such CHECK over a field a client sends is mapped.
func TestEveryDealPicklistIsAPicklistCheckOnTheDeal(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	sent := wireKeys(reflect.TypeFor[crmcontracts.CreateDealRequest](), reflect.TypeFor[crmcontracts.UpdateDealRequest]())
	checks := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := dealPicklistCheck.FindStringSubmatch(line); m != nil {
			checks[m[1]] = m[2]
		}
	}
	if len(checks) == 0 {
		t.Fatal("the catalog shows no picklist CHECK on the deal: the scan stopped seeing them")
	}
	for constraint, field := range dealPicklists {
		if checks[constraint] != field {
			t.Errorf("dealPicklists maps %s to %q; the catalog holds %q to its options", constraint, field, checks[constraint])
		}
	}
	for constraint, column := range checks {
		if sent[column] && dealPicklists[constraint] != column {
			t.Errorf("%s holds the client field %q to its options and dealPicklists does not name it", constraint, column)
		}
	}
}

func wireKeys(requests ...reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for _, typ := range requests {
		for field := range typ.Fields() {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			keys[name] = true
		}
	}
	return keys
}
