// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"reflect"
	"strings"
	"testing"
)

func TestSidecarImagesStateTheCompanyFieldBeforeAndAfter(t *testing.T) {
	before := map[string]any{"value": "abc", "source": "site_read"}
	after := map[string]any{"value": "DE123456789", "source": "human"}

	gotBefore, gotAfter, action := sidecarImages("register_vat", "update", before, after)

	if !reflect.DeepEqual(gotBefore, map[string]any{"register_vat": "abc"}) ||
		!reflect.DeepEqual(gotAfter, map[string]any{"register_vat": "DE123456789"}) || action != "update" {
		t.Errorf("got %v -> %v as %q, want the field's value moving from abc to DE123456789 as an update", gotBefore, gotAfter, action)
	}
}

func TestSidecarImagesReadACreationAsAnUpdateFromNothing(t *testing.T) {
	gotBefore, gotAfter, action := sidecarImages("registered_address", "create", nil,
		map[string]any{"value": "Musterstrasse 1, 10115 Berlin"})

	if gotBefore != nil || gotAfter["registered_address"] != "Musterstrasse 1, 10115 Berlin" || action != "update" {
		t.Errorf("got %v -> %v as %q, want a first value shown as an update", gotBefore, gotAfter, action)
	}
}

func TestSidecarImagesShowNoChangeForAConfirmation(t *testing.T) {
	gotBefore, gotAfter, _ := sidecarImages("register_vat", "update",
		map[string]any{"value": "DE123456789"}, map[string]any{"verified_by": "someone"})

	if len(gotBefore) != 0 || len(gotAfter) != 0 {
		t.Errorf("a confirmation that moved no value shows %v -> %v", gotBefore, gotAfter)
	}
}

func TestSidecarImagesShowARemovalAsTheValueGoing(t *testing.T) {
	gotBefore, gotAfter, action := sidecarImages("employee_range", "delete", map[string]any{"value": "11-50"}, nil)

	if gotBefore["employee_range"] != "11-50" || gotAfter != nil || action != "update" {
		t.Errorf("got %v -> %v as %q, want 11-50 removed as an update", gotBefore, gotAfter, action)
	}
}

func TestOnlyACompanyHasSidecarRows(t *testing.T) {
	if sidecarSubjectCTE("deal", 2) != "" || sidecarSubjectCTE("contact", 2) != "" {
		t.Error("a record kind without sidecar tables got a sidecar branch")
	}
	cte := sidecarSubjectCTE(entityTypeCompany, 2)
	for _, table := range []string{sidecarProfileFieldTable, sidecarFactTable} {
		if !strings.Contains(cte, "audit_log a ON a.entity_type = '"+table+"'") {
			t.Errorf("the sidecar branch does not read the audit rows of %s", table)
		}
	}
}
