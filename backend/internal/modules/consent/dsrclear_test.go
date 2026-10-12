// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
			UpdateDSRInput{Status: new("rejected"), ClearResolution: true},
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

// failingNames stands for a name read that errors after the write committed.
type failingNames struct{}

func (failingNames) Labels(context.Context, string, []ids.UUID) (map[ids.UUID]string, error) {
	return nil, errors.New("names unavailable")
}

func TestAFailedNameReadStillAnswersTheWrite(t *testing.T) {
	h := Handlers{}.WithRecordNames(failingNames{})
	subject := ids.NewV7()
	answers := map[string]func(http.ResponseWriter, *http.Request){
		"a subject request": func(w http.ResponseWriter, r *http.Request) {
			h.writeDSR(w, r, http.StatusOK, dsrRow{ID: ids.NewV7(), SubjectRef: subject.String()})
		},
		"a disclosure duty": func(w http.ResponseWriter, r *http.Request) {
			h.writeNoticeCase(w, r, NoticeCase{ID: ids.NewV7(), ContactID: ids.From[ids.ContactKind](subject)})
		},
	}
	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			answer(w, httptest.NewRequest(http.MethodPatch, "/", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("a committed write answered %d %s, want 200: a retry would write it twice", w.Code, w.Body)
			}
		})
	}
}
