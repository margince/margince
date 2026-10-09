// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestClearingAnAnswerIsRefusedWhereTheRequestEndsClosed(t *testing.T) {
	answer := "package sent"
	cases := []struct {
		name    string
		current dsrRow
		in      UpdateDSRInput
		refused bool
	}{
		{"an open request drops its draft", dsrRow{Status: "open", Resolution: &answer}, UpdateDSRInput{ClearResolution: true}, false},
		{"a fulfilled request keeps its answer", dsrRow{Status: "fulfilled", Resolution: &answer}, UpdateDSRInput{ClearResolution: true}, true},
		{
			"closing while clearing has no answer",
			dsrRow{Status: "open", Resolution: &answer},
			UpdateDSRInput{Status: strptrUnit("rejected"), ClearResolution: true},
			true,
		},
		{
			"handing a closed request back is no answer change",
			dsrRow{Status: "fulfilled", Resolution: &answer},
			UpdateDSRInput{ClearAssignee: true},
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verr := validateDSRUpdate(c.current, c.in)
			if (verr != nil) != c.refused {
				t.Fatalf("validateDSRUpdate = %v, want refused=%v", verr, c.refused)
			}
			if verr != nil && verr.Field != fieldResolution {
				t.Errorf("refused on %q, want %q", verr.Field, fieldResolution)
			}
		})
	}
}

func TestANameTheReaderMayNotSeeIsNull(t *testing.T) {
	seen, blank, hidden := ids.NewV7(), ids.NewV7(), ids.NewV7()
	labels := map[ids.UUID]string{seen: "Ada", blank: ""}
	if got := labelOf(labels, seen); got == nil || *got != "Ada" {
		t.Errorf("a readable name answered %v", got)
	}
	if got := labelOf(labels, blank); got != nil {
		t.Errorf("a record with no name answered %q, want null", *got)
	}
	if got := labelOf(labels, hidden); got != nil {
		t.Errorf("a withheld record answered %q, want null", *got)
	}
	var none *NoticeAcquisition
	if none.Wire() != nil {
		t.Error("a duty with no evidence put an acquisition on the wire")
	}
}

func strptrUnit(s string) *string { return &s }
