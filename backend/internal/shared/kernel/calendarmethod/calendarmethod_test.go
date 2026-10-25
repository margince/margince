// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package calendarmethod_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/calendarmethod"
)

func TestOnlyTheRFCMethodsAreKnown(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"reply": "REPLY", "\tDeclineCounter\n": "DECLINECOUNTER", "X-ANSWER": "", "": "",
	} {
		if got := calendarmethod.Known(in); got != want {
			t.Errorf("Known(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOnlyAnAttendeesAnswerIsAResponse(t *testing.T) {
	t.Parallel()
	for method, want := range map[string]bool{
		"REPLY": true, "COUNTER": true, "DECLINECOUNTER": true,
		"REQUEST": false, "CANCEL": false, "PUBLISH": false, "": false,
	} {
		if got := calendarmethod.IsResponse(method); got != want {
			t.Errorf("IsResponse(%q) = %v, want %v", method, got, want)
		}
	}
}

func TestEveryKnownMethodIsAResponseOrAnInvitation(t *testing.T) {
	t.Parallel()
	listed := append(calendarmethod.Responses(), calendarmethod.Invitations()...)
	for _, method := range []string{"PUBLISH", "REQUEST", "REPLY", "ADD", "CANCEL", "REFRESH", "COUNTER", "DECLINECOUNTER"} {
		if calendarmethod.Known(method) != method {
			t.Fatalf("%q is not known", method)
		}
		if calendarmethod.IsResponse(method) == calendarmethod.IsInvitation(method) {
			t.Errorf("%q: IsResponse %v, IsInvitation %v; want exactly one",
				method, calendarmethod.IsResponse(method), calendarmethod.IsInvitation(method))
		}
	}
	if len(listed) != 8 {
		t.Errorf("the two lists hold %d methods, want the eight RFC 5546 defines", len(listed))
	}
	for _, method := range listed {
		if calendarmethod.Known(method) != method {
			t.Errorf("listed method %q is not known", method)
		}
	}
	if calendarmethod.IsInvitation("") || calendarmethod.IsResponse("") {
		t.Error("an absent method is neither a response nor an invitation")
	}
}
