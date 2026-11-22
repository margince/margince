// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func notesRecord(from, kind string, authenticated bool) connector.NormalizedRecord {
	rec := connector.NormalizedRecord{
		CapturedBy:   "connector:" + providerGmail,
		Counterparty: connector.Counterparty{Email: from},
		Fields:       ActivityFields{Kind: kind, Direction: connector.DirectionInbound},
	}
	if authenticated {
		rec.SenderAuthentication = connector.SenderAuthentication{Server: gmailAuthServer, DMARCPassDomain: "google.com"}
	}
	return rec
}

// Only Google's two mails about a recorded meeting hold, and only when Gmail
// authenticated the sender. A calendar invitation is not notes.
func TestOnlyAuthenticatedGoogleMeetingNotesMailIsHeld(t *testing.T) {
	for _, test := range []struct {
		name          string
		from, kind    string
		authenticated bool
		want          bool
	}{
		{"the notes Gemini wrote", geminiNotesSender, kindEmail, true, true},
		{"the list of the meeting's records", meetingRecordsSender, kindEmail, true, true},
		{"a calendar invitation", "calendar-notification@google.com", kindEmail, true, false},
		{"a forged notes From", geminiNotesSender, kindEmail, false, false},
		{"another google.com address", "sales-team@google.com", kindEmail, true, false},
		{"a calendar meeting record", geminiNotesSender, meetingKind, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := notesRecord(test.from, test.kind, test.authenticated)
			if got := isMeetingNotesMail(rec); got != test.want {
				t.Errorf("isMeetingNotesMail = %v, want %v", got, test.want)
			}
			held := withNotesMailHold(rec, counterpartyDecision{create: true, linkOnly: true})
			if held.notesMail != test.want || (test.want && (held.create || held.linkOnly)) {
				t.Errorf("hold = %+v, want a notes mail that creates and links nothing: %v", held, test.want)
			}
		})
	}
}

func TestNotesTextDropsTheFromAndToLinesTheMapperAdds(t *testing.T) {
	for body, want := range map[string]string{
		"From: gemini-notes@google.com\nTo: me@x.example\n\nSummary\nAda sends it.": "Summary\nAda sends it.",
		"From: gemini-notes@google.com\nTo: me@x.example":                           "",
		"Summary only": "Summary only",
	} {
		if got := notesText(body); got != want {
			t.Errorf("notesText(%q) = %q, want %q", body, got, want)
		}
	}
}

// The hold on a notes mail does not open when a record is later linked to it,
// whatever reason the ladder gave. A ladder reason of no counterparty would.
func TestANotesMailIsHeldAsNoRecordWhateverTheLadderSaid(t *testing.T) {
	ladder := counterpartyDecision{traceReason: traceReasonNoCounterparty}
	if got, err := linkLessReason(t.Context(), nil, connector.NormalizedRecord{}, meetingKind, ladder); err != nil || got != audienceReasonNoCounterparty {
		t.Fatalf("an ordinary link-less record is held as %q (%v), want no_counterparty", got, err)
	}
	ladder.notesMail = true
	if got, err := linkLessReason(t.Context(), nil, connector.NormalizedRecord{}, kindEmail, ladder); err != nil || got != audienceReasonNoRecord {
		t.Errorf("a notes mail is held as %q (%v), want no_record", got, err)
	}
}
