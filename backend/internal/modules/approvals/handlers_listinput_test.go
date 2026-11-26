// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"net/http"
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheInboxClampsAnOutOfRangeLimitLikeEveryOtherList(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit *int
		want  int
	}{
		{"absent", nil, 50},
		{"zero", new(0), 1},
		{"negative", new(-1), 1},
		{"one", new(1), 1},
		{"in range", new(25), 25},
		{"max", new(200), 200},
		{"over", new(201), 200},
		{"far over", new(1000), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := crmcontracts.ListApprovalsParams{}
			if tc.limit != nil {
				l := crmcontracts.Limit(*tc.limit)
				params.Limit = &l
			}
			in, err := listInput(params)
			if err != nil {
				t.Fatalf("listInput: %v", err)
			}
			if in.Limit != tc.want {
				t.Fatalf("limit %v bound as %d, want %d", tc.limit, in.Limit, tc.want)
			}
		})
	}
}

func TestTheInboxReadsEveryRepeatedTargetAsOneRecord(t *testing.T) {
	contact, activity := ids.NewV7(), ids.NewV7()
	in, err := listInput(crmcontracts.ListApprovalsParams{Target: &[]string{
		"contact:" + contact.String(), "activity:" + activity.String(),
	}})
	if err != nil {
		t.Fatalf("listInput: %v", err)
	}
	want := []ids.Ref{{Type: "contact", ID: contact}, {Type: "activity", ID: activity}}
	if !slices.Equal(in.Targets, want) {
		t.Fatalf("targets bound as %v, want %v", in.Targets, want)
	}
	if in.targeted() {
		t.Error("a target list took the one-record read, which skips the inbox's paging")
	}
}

func TestTheInboxRefusesATargetItNamesNoRecordWith(t *testing.T) {
	recordID := ids.NewV7().String()
	tooMany := make([]string, maxTargets+1)
	for i := range tooMany {
		tooMany[i] = "contact:" + recordID
	}
	for name, items := range map[string][]string{
		"no separator":   {"contact" + recordID},
		"no type":        {":" + recordID},
		"not a uuid":     {"contact:42"},
		"one bad of two": {"contact:" + recordID, "deal:"},
		"over the cap":   tooMany,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := listInput(crmcontracts.ListApprovalsParams{Target: &items})
			if err == nil || err.Status != http.StatusUnprocessableEntity || err.Fields[0].Field != "target" {
				t.Fatalf("listInput(%v) = %v, want a 422 on target", items, err)
			}
		})
	}
}

func TestTheInboxRefusesATargetListBesideTheTargetPair(t *testing.T) {
	recordID := ids.NewV7()
	entityType := "contact"
	pairID := openapi_types.UUID(recordID)
	_, err := listInput(crmcontracts.ListApprovalsParams{
		TargetEntityType: &entityType, TargetEntityId: &pairID,
		Target: &[]string{"deal:" + ids.NewV7().String()},
	})
	if err == nil || err.Status != http.StatusUnprocessableEntity || err.Fields[0].Field != "target" {
		t.Fatalf("listInput = %v, want a 422 on target", err)
	}
}
