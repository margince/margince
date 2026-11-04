// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func TestAFieldNarrowedToMeetingsRefusesAValueOnACallAndClearsOnAny(t *testing.T) {
	columns := []fieldcatalog.Column{
		{Name: "cf_visit_report", Type: fieldcatalog.TypeText, ActivityKinds: []string{KindMeeting}},
		{Name: "cf_region", Type: fieldcatalog.TypeText},
	}
	cases := []struct {
		name   string
		kind   string
		values map[string]any
		refuse string
	}{
		{"a meeting carries its field", KindMeeting, map[string]any{"cf_visit_report": "On site"}, ""},
		{"a call refuses it", "call", map[string]any{"cf_visit_report": "On site"}, "cf_visit_report"},
		{"a call clears it", "call", map[string]any{"cf_visit_report": nil}, ""},
		{"a field for every kind fits a call", "call", map[string]any{"cf_region": "North"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkCustomValues(tc.kind, columns, tc.values)
			kindErr, refused := errors.AsType[*KindFieldError](err)
			if tc.refuse == "" && err != nil {
				t.Fatalf("want no refusal, got %v", err)
			}
			if tc.refuse != "" && (!refused || kindErr.Field != tc.refuse) {
				t.Fatalf("want field_not_valid_for_kind on %s, got %v", tc.refuse, err)
			}
		})
	}
}

func TestTheKindRefusalNamesWhereTheValueBelongs(t *testing.T) {
	err := checkCustomValues("note", []fieldcatalog.Column{
		{Name: "cf_visit_report", Type: fieldcatalog.TypeText, ActivityKinds: []string{"call", KindMeeting}},
	}, map[string]any{"cf_visit_report": "x"})
	kindErr, ok := errors.AsType[*KindFieldError](err)
	if !ok {
		t.Fatalf("want a KindFieldError, got %v", err)
	}
	field, code, message := kindErr.FieldFault()
	if field != "cf_visit_report" || code != faultNotValidForKind || message != "only a call or a meeting may carry cf_visit_report" {
		t.Fatalf("got %s/%s %q", field, code, message)
	}
}
