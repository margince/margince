// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Evidence for a withheld owner opens nothing, and an ungrouped page names no
// withheld owner on its rows, so neither door lists one colleague's work.
func TestEvidenceKeepsAWithheldOwnerOutOfItsGroupAndItsRows(t *testing.T) {
	reader, colleague := ids.NewV7(), ids.NewV7()
	readerID, colleagueID := openapi_types.UUID(reader), openapi_types.UUID(colleague)
	facts := []Fact{
		{Metric: "bookings_won", ContextID: "interval", OwnerID: reader, Row: crmcontracts.ReportingEvidenceRow{Key: "mine", OwnerId: &readerID}},
		{Metric: "bookings_won", ContextID: "interval", OwnerID: colleague, OwnerWithheld: true, Row: crmcontracts.ReportingEvidenceRow{Key: "theirs", OwnerId: &colleagueID}},
	}
	grouped, err := evidencePage(crmcontracts.ReportingContext{}, facts, "bookings_won", "interval", "owner:"+colleague.String(), nil, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped.Rows) != 0 {
		t.Errorf("evidence for a withheld owner opened %d rows, want none", len(grouped.Rows))
	}
	all, err := evidencePage(crmcontracts.ReportingContext{}, facts, "bookings_won", "interval", "", nil, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Rows) != 2 || all.Rows[0].OwnerId == nil || all.Rows[1].OwnerId != nil {
		t.Errorf("ungrouped evidence rows = %+v, want both rows, the withheld one with no owner", all.Rows)
	}
}

// Each reach binds the values its clause reads and no others: a value bound
// and never read fails the statement.
func TestTargetReachBindsWhatItsClauseReads(t *testing.T) {
	self := ids.NewV7()
	for name, reach := range map[string]targetReach{
		"every target":           {every: true, owners: MeasurableOwners{Every: true}},
		"switch off, every team": {every: true, owners: MeasurableOwners{Members: []ids.UUID{self}, IndividualOff: true}},
		"a lead":                 {owners: MeasurableOwners{Members: []ids.UUID{self}}, teams: []ids.UUID{ids.NewV7()}},
		"switch off, a floor": {
			every: true, owners: MeasurableOwners{Members: []ids.UUID{self}, IndividualOff: true},
			floor: TargetFloor{Applies: true, Teams: []ids.UUID{ids.NewV7()}},
		},
	} {
		var b bindings
		clause := reach.clause(&b)
		if placeholders := strings.Count(clause, "$"); placeholders != len(b.values) {
			t.Errorf("%s: clause %q reads %d values and binds %d", name, clause, placeholders, len(b.values))
		}
		if reach.owners.IndividualOff && !strings.Contains(clause, "scope_kind<>'owner'") {
			t.Errorf("%s: clause %q lets every owner target through", name, clause)
		}
	}
}
