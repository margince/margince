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
