// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an import may be mapped to.

import (
	"strings"
	"testing"
)

// Nothing the importer offers as a target is a custom field.
//
// An import lands through the stores' caller-opened transaction seams, which
// refuse custom fields by design — reading the catalog is the second connection
// those seams exist to avoid. So a `cf_` target would be accepted, reported as
// written, and dropped: the worst shape a mapping can take, because the report
// says the column landed.
//
// The corpus is every object that HAS a target table rather than three names
// typed here, so a fourth importable object is examined the day it is added,
// and `gates/importtargetsclaim_test.go` holds the contract to the same answer.
func TestNoImportTargetIsACustomField(t *testing.T) {
	if len(csvTargets) == 0 {
		t.Fatal("no importable objects, so this examined nothing")
	}
	for object := range csvTargets {
		targets, err := importTargets(object)
		if err != nil {
			t.Fatalf("%s has no mappable fields: %v", object, err)
		}
		if len(targets) == 0 {
			t.Errorf("%s offers no targets at all, so a mapping for it can name nothing", object)
		}
		for _, target := range targets {
			if strings.HasPrefix(target, "cf_") {
				t.Errorf("%s offers %q, and an import writes through a seam that refuses custom "+
					"fields — the column would be reported as written and dropped", object, target)
			}
		}
	}
}
