// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Citations carries the whole plan, and this is what says so.
//
// The read bound charges what a tool call reached. A field added to the plan
// whose citations Citations does not walk is a set of records an agent read for
// free — invisible, because nothing else in the tree counts them. So rather
// than trusting the method to keep up, this walks the STRUCT: every field that
// can hold evidence must be one Citations reports.
func TestCitationsReachesEveryFieldThatCanCarryEvidence(t *testing.T) {
	// One citation per evidence-bearing field, each with its own record id, so
	// a field the method skips is a specific id that never arrives.
	var planted []ids.UUID
	cite := func() []MeetingBriefCite {
		id := ids.NewV7()
		planted = append(planted, id)
		return []MeetingBriefCite{{RecordType: "activity", RecordID: id}}
	}
	line := func() MeetingBriefLine {
		return MeetingBriefLine{Text: "x", Evidence: cite()}
	}
	plan := MeetingPlanResult{
		Objective: ptrTo(line()),
		Opening:   ptrTo(line()),
		TopRisk: &MeetingPlanRiskPart{
			Text: line(),
		},
		LikelyAsks: []MeetingPlanAskPart{
			{Basis: line()},
		},
		Questions: []MeetingPlanAskLine{
			{Evidence: cite()},
		},
		Scenarios: []MeetingPlanBranch{
			{Evidence: cite()},
		},
		Arc: []MeetingPlanMoment{
			{Summary: line()},
		},
		Advance: MeetingPlanAdvancePart{
			Minimum:  line(),
			Best:     line(),
			Fallback: line(),
		},
	}

	got := map[ids.UUID]bool{}
	for _, cited := range plan.Citations() {
		got[cited.RecordID] = true
	}
	for i, want := range planted {
		if !got[want] {
			t.Errorf("Citations did not report the record planted in field %d; "+
				"a field it does not walk is records an agent reads without being charged", i)
		}
	}

	// And the census the claim rests on: every field of the struct that can
	// hold evidence is one of the ones exercised above. A new one fails here
	// rather than going uncounted in production.
	evidenceBearing := []string{
		"Objective", "Opening", "TopRisk", "LikelyAsks",
		"Questions", "Scenarios", "Arc", "Advance",
	}
	shape := reflect.TypeOf(MeetingPlanResult{})
	for i := range shape.NumField() {
		field := shape.Field(i)
		if !canCarryEvidence(field.Type) {
			continue
		}
		if !contains(evidenceBearing, field.Name) {
			t.Errorf("field %q can carry evidence and is not in this test's census; "+
				"add it here and to Citations, or an agent reads it for free", field.Name)
		}
	}
}

// canCarryEvidence reports whether a field's type reaches a MeetingBriefCite.
func canCarryEvidence(shape reflect.Type) bool {
	switch shape.Kind() {
	case reflect.Pointer, reflect.Slice:
		return canCarryEvidence(shape.Elem())
	case reflect.Struct:
		for i := range shape.NumField() {
			if canCarryEvidence(shape.Field(i).Type) {
				return true
			}
		}
		return false
	default:
		return shape.String() == "[]agents.MeetingBriefCite" ||
			strings.HasSuffix(shape.String(), "MeetingBriefCite")
	}
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func ptrTo[T any](v T) *T { return &v }
