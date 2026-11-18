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

	gotBefore, gotAfter := sidecarImages("register_vat", "update", before, after)

	if !reflect.DeepEqual(gotBefore, map[string]any{"register_vat": "abc"}) ||
		!reflect.DeepEqual(gotAfter, map[string]any{"register_vat": "DE123456789"}) {
		t.Errorf("got %v -> %v, want the field's value moving from abc to DE123456789", gotBefore, gotAfter)
	}
}

func TestSidecarImagesReadACreationAsAnUpdateFromNothing(t *testing.T) {
	gotBefore, gotAfter := sidecarImages("registered_address", "create", nil,
		map[string]any{"value": "Musterstrasse 1, 10115 Berlin"})

	if gotBefore != nil || gotAfter["registered_address"] != "Musterstrasse 1, 10115 Berlin" {
		t.Errorf("got %v -> %v, want a first value with nothing before it", gotBefore, gotAfter)
	}
}

func TestSidecarImagesShowNoChangeForAConfirmation(t *testing.T) {
	gotBefore, gotAfter := sidecarImages("register_vat", "update",
		map[string]any{"value": "DE123456789"}, map[string]any{"verified_by": "someone"})

	if len(gotBefore) != 0 || len(gotAfter) != 0 {
		t.Errorf("a confirmation that moved no value shows %v -> %v", gotBefore, gotAfter)
	}
}

func TestSidecarImagesShowARemovalAsTheValueGoing(t *testing.T) {
	gotBefore, gotAfter := sidecarImages("employee_range", "delete", map[string]any{"value": "11-50"}, nil)

	if gotBefore["employee_range"] != "11-50" || gotAfter != nil {
		t.Errorf("got %v -> %v, want 11-50 removed", gotBefore, gotAfter)
	}
}

func TestOnlyACompanyHasSidecarRows(t *testing.T) {
	if sidecarSubjectCTE("deal", 2) != "" || sidecarSubjectCTE("contact", 2) != "" {
		t.Error("a record kind without sidecar tables got a sidecar branch")
	}
	cte := sidecarSubjectCTE(entityTypeCompany, 2)
	for _, table := range []string{"company_profile_field", "company_fact"} {
		if !strings.Contains(cte, "audit_log a ON a.entity_type = '"+table+"'") {
			t.Errorf("the sidecar branch does not read the audit rows of %s", table)
		}
	}
}
