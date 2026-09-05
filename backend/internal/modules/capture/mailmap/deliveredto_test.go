// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"
	"testing"

	"github.com/emersion/go-message/mail"
)

// headerOf parses a header block written the way a real message carries it,
// top line first, so a case's ORDER is the thing under test.
func headerOf(t *testing.T, lines ...string) mail.Header {
	t.Helper()
	raw := strings.Join(lines, "\r\n") + "\r\n\r\nbody\r\n"
	reader, err := mail.CreateReader(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parsing the probe message: %v", err)
	}
	return reader.Header
}

// The refusals are the point of this table, so they outnumber the acceptance.
// A missed alias costs a contact somebody has to delete; a trusted forgery
// lets a sender declare themselves the mailbox owner.
func TestOnlyADeliveryAboveEveryHopIsTrusted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name: "written by the delivering server, above every hop",
			lines: []string{
				"Delivered-To: alias@founder.example",
				"Received: from mx by google",
				"From: someone@elsewhere.example",
			},
			want: "alias@founder.example",
		},
		{
			name: "the first header of all, which is where Gmail writes it",
			lines: []string{
				"Delivered-To: alias@founder.example",
				"From: someone@elsewhere.example",
			},
			want: "alias@founder.example",
		},
		{
			// The attack. A sender submits a Delivered-To naming the person
			// they want silenced; the receiving hop's Received lands above it.
			name: "below a hop, where the sender could have written it",
			lines: []string{
				"Received: from mx by google",
				"Delivered-To: victim@customer.example",
				"From: attacker@elsewhere.example",
			},
			want: "",
		},
		{
			// A real delivery AND a forged one. The real one is on top, so the
			// answer is the real one — and a walk that read the LAST match, or
			// collected them all, would answer with the forgery.
			name: "a real delivery above a forged one",
			lines: []string{
				"Delivered-To: alias@founder.example",
				"Received: from mx by google",
				"Delivered-To: victim@customer.example",
				"From: attacker@elsewhere.example",
			},
			want: "alias@founder.example",
		},
		{
			name: "no delivery claim at all",
			lines: []string{
				"Received: from mx by google",
				"From: someone@elsewhere.example",
			},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TopDeliveredTo(headerOf(t, tc.lines...)); got != tc.want {
				t.Errorf("TopDeliveredTo = %q, want %q", got, tc.want)
			}
		})
	}
}
