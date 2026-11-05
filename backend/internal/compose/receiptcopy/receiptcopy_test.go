// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package receiptcopy

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestTheSentOriginNamesTheChannelWeSpokeOn(t *testing.T) {
	for kind, want := range map[string]string{
		"email":   "Email you sent",
		"message": "Message you sent",
		"call":    "Call you made",
		"meeting": "Meeting you held",
		"fax":     "Sent by you",
	} {
		if got := Origin(langcopy.For("en"), kind, "outbound"); got != want {
			t.Errorf("sentOrigin(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestReceiptLanguagesCoverEveryChannel(t *testing.T) {
	for kind, words := range channels {
		t.Run(kind, func(t *testing.T) { langcopytest.Census(t, words) })
	}
}

func TestReceiptDirectionDoesNotTurnCallsIntoSentMessages(t *testing.T) {
	for _, tc := range []struct{ kind, direction, lang, want string }{
		{"call", "inbound", "en", "Call they made"},
		{"meeting", "", "en", "Meeting"},
		{"email", "inbound", "de", "Empfangene E-Mail"},
		{"email", "outbound", "vi", "Email bạn đã gửi"},
		{"fax", "inbound", "en", "Sent by them"},
		{"fax", "", "en", "Exchange"},
	} {
		if got := Origin(langcopy.For(tc.lang), tc.kind, tc.direction); got != tc.want {
			t.Errorf("%+v: got %q", tc, got)
		}
	}
}
