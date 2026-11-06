// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func ownerPoint(owner openapi_types.UUID, label string) crmcontracts.ReportingPoint {
	group := ownerGroupPrefix + owner.String()
	return crmcontracts.ReportingPoint{Key: owner.String(), Label: label, Evidence: &crmcontracts.ReportingEvidenceRef{GroupKey: &group}}
}

func TestTwoColleaguesWithOneNameEachCarryTheirEmail(t *testing.T) {
	first, second, other := openapi_types.UUID(ids.NewV7()), openapi_types.UUID(ids.NewV7()), openapi_types.UUID(ids.NewV7())
	charts := []crmcontracts.ReportingChart{{Points: []crmcontracts.ReportingPoint{
		ownerPoint(first, "Joschua Ziethen"),
		ownerPoint(second, "Joschua Ziethen"),
		ownerPoint(other, "Darius Ziethen"),
	}}}

	shared := sharedOwnerNames(charts)
	if len(shared) != 2 {
		t.Fatalf("shared names = %v, want the two Joschuas", shared)
	}
	labelSharedOwners(charts, map[openapi_types.UUID]string{
		first: "joschua@one.test", second: "joschua@two.test",
	})

	got := []string{charts[0].Points[0].Label, charts[0].Points[1].Label, charts[0].Points[2].Label}
	want := []string{"Joschua Ziethen · joschua@one.test", "Joschua Ziethen · joschua@two.test", "Darius Ziethen"}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("label %d = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestAStageChartWithRepeatedLabelsAsksForNoEmail(t *testing.T) {
	group := "stage:1"
	charts := []crmcontracts.ReportingChart{{Points: []crmcontracts.ReportingPoint{
		{Label: "Proposal", Evidence: &crmcontracts.ReportingEvidenceRef{GroupKey: &group}},
		{Label: "Proposal", Evidence: &crmcontracts.ReportingEvidenceRef{GroupKey: &group}},
	}}}
	if shared := sharedOwnerNames(charts); len(shared) != 0 {
		t.Fatalf("shared = %v, want none for stages", shared)
	}
}
