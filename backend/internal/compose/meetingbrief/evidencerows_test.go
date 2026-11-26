// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"reflect"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var evidenceType = reflect.TypeFor[crmcontracts.CompanyBriefEvidence]()

// fillEveryCitation sets every pointer and gives every slice one element, so
// each place the contract can carry a citation carries one, and counts them.
func fillEveryCitation(value reflect.Value) int {
	switch value.Kind() {
	case reflect.Pointer:
		value.Set(reflect.New(value.Type().Elem()))
		return fillEveryCitation(value.Elem())
	case reflect.Slice:
		value.Set(reflect.MakeSlice(value.Type(), 1, 1))
		return fillEveryCitation(value.Index(0))
	case reflect.Struct:
		if value.Type() == evidenceType {
			value.Set(reflect.ValueOf(crmcontracts.CompanyBriefEvidence{
				EntityType: crmcontracts.CompanyBriefEvidenceEntityTypeActivity,
				EntityId:   openapi_types.UUID(ids.MustParse(activityID)),
			}))
			return 1
		}
		count := 0
		for i := range value.NumField() {
			if value.Type().Field(i).IsExported() {
				count += fillEveryCitation(value.Field(i))
			}
		}
		return count
	default:
		return 0
	}
}

// The census comes from the contract, so a citation added to the brief later
// fails here until the walk reaches it.
func TestEveryCitationInTheBriefIsWalked(t *testing.T) {
	var brief crmcontracts.MeetingBrief
	want := fillEveryCitation(reflect.ValueOf(&brief).Elem())
	if got := len(meetingCitations(&brief)); got != want || want == 0 {
		t.Fatalf("walked %d citations, the contract holds %d", got, want)
	}
}

func TestACitedActivityCarriesItsSubjectAndTime(t *testing.T) {
	in := fullInput()
	var brief crmcontracts.MeetingBrief
	fillEveryCitation(reflect.ValueOf(&brief).Elem())
	citations := meetingCitations(&brief)
	nameCitations(citations, in)
	for _, cited := range citations {
		if cited.Name == nil || *cited.Name != "Re: security review" {
			t.Fatalf("citation not named by its subject: %+v", cited)
		}
		if cited.At == nil || !cited.At.Equal(in.Recent[0].At) {
			t.Fatalf("citation not dated: %+v", cited)
		}
	}
}

func TestAWithheldConversationKeepsItsSubject(t *testing.T) {
	in := fullInput()
	in.Recent, in.Commitments = nil, nil
	in.History = []HistoryIn{{ID: activityID, Subject: "Private thread", At: at(3), Withheld: true}}
	var brief crmcontracts.MeetingBrief
	fillEveryCitation(reflect.ValueOf(&brief).Elem())
	citations := meetingCitations(&brief)
	nameCitations(citations, in)
	for _, cited := range citations {
		if cited.Name != nil || cited.At != nil {
			t.Fatalf("withheld conversation leaked into a citation: %+v", cited)
		}
	}
}
