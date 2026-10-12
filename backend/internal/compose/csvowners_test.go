// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUnknownOwnerFallsBackToTheImporterWithAWarning(t *testing.T) {
	o := &ownerResolver{seats: map[string]ids.UUID{"ann@x.test": ids.NewV7()}}
	if id, warn := o.ownerFor("Ann@X.test"); id == nil || warn != nil {
		t.Fatalf("known owner: %v %v", id, warn)
	}
	id, warn := o.ownerFor("mai@old-crm.example")
	if id != nil || warn == nil || warn.Kind != migration.ProblemUnknownOwner {
		t.Fatalf("unknown owner: id %v warning %+v", id, warn)
	}
	if warn.Value != "mai@old-crm.example" {
		t.Fatalf("warning value = %q, want the address as written", warn.Value)
	}
}

func TestARowsOwnerIsComparedByTheResolvedSeat(t *testing.T) {
	ann := ids.NewV7()
	stored := []byte(`{"owner_id":"` + ann.String() + `"}`)
	fields := map[string]string{csvTargetOwner: "ann@x.test"}
	changed, err := changedFields(stored, fields, map[string]any{csvTargetOwner: ann})
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v err %v, want none: the record already has this owner", changed, err)
	}
	changed, err = changedFields(stored, map[string]string{csvTargetOwner: "mai@old-crm.example"}, nil)
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v err %v, want none: an unknown owner leaves the record's owner alone", changed, err)
	}
	changed, err = changedFields(stored, fields, map[string]any{csvTargetOwner: ids.NewV7()})
	if err != nil || len(changed) != 1 {
		t.Fatalf("changed = %v err %v, want the owner moved", changed, err)
	}
}
