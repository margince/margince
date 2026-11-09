// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/erasureaccount"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestChangingAnAnswerIsRefusedWhereTheRequestEndsClosed(t *testing.T) {
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
		{"a fulfilled request refuses another answer", dsrRow{Status: "fulfilled", Resolution: &answer}, UpdateDSRInput{Resolution: new("rewritten")}, true},
		{"a fulfilled request takes its own answer again", dsrRow{Status: "fulfilled", Resolution: &answer}, UpdateDSRInput{Resolution: new(answer)}, false},
		{"an open request rewrites its draft", dsrRow{Status: "open", Resolution: &answer}, UpdateDSRInput{Resolution: new("redrafted")}, false},
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

func TestAReplayedErasureIsNotARewrite(t *testing.T) {
	erased := erasedResolution
	closed := dsrRow{Status: "fulfilled", Resolution: &erased}
	echo := UpdateDSRInput{Status: new("fulfilled"), Resolution: new(erasedResolution)}

	if got := withoutReplayedAnswer(closed, echo); got.Resolution != nil {
		t.Errorf("a replay echoing the recorded answer kept it as new text: %v", *got.Resolution)
	}
	// Prose is not dropped here: FulfilErasure refuses it with a 422 instead.
	prose := UpdateDSRInput{Status: new("fulfilled"), Resolution: new("sent the export")}
	if got := withoutReplayedAnswer(closed, prose); got.Resolution == nil {
		t.Error("prose on a replay was dropped without a word")
	}
	open := dsrRow{Status: "in_progress"}
	if got := withErasureAnswer(open, UpdateDSRInput{Status: new("fulfilled")}); validateDSRUpdate(open, got) != nil {
		t.Error("a first erasure fulfilment with no typed answer was refused")
	}
}

// The erasure's answer comes from its own account. Rows it only inserted are
// its records of the work, not something it found.
func TestAnErasureAnswersFromWhatItFound(t *testing.T) {
	cases := []struct {
		name string
		acct erasureaccount.Account
		want string
	}{
		{"nothing at all", erasureaccount.Empty(), erasureAnswerNothingHeld},
		{"all zero", erasureaccount.Account{Rows: map[string]int64{"contact.updated": 0, "activity.deleted": 0}}, erasureAnswerNothingHeld},
		{"only its own records", erasureaccount.Account{Rows: map[string]int64{"erasure_tombstone.inserted": 3}}, erasureAnswerNothingHeld},
		{"one row updated", erasureaccount.Account{Rows: map[string]int64{"contact.updated": 1}}, erasedResolution},
		{"one row deleted", erasureaccount.Account{Rows: map[string]int64{"activity.deleted": 4}}, erasedResolution},
		{"only outside the database", erasureaccount.Account{Rows: map[string]int64{}, Uncounted: []string{"blob"}}, erasedResolution},
	}
	for _, c := range cases {
		if got := erasureAnswerFor(c.acct); got != c.want {
			t.Errorf("%s: answer = %q, want %q", c.name, got, c.want)
		}
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
