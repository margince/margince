// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The summary is the one part of an approval that is prose, and several
// stagers build it out of record text an agent or an inbound sender wrote:
// a display name arrives from captured mail, and creating a contact is
// auto-execute for an agent. So the text a human reads before deciding is
// attacker-influenced, and these cases pin what may reach their screen.

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestSanitizeSummaryStripsWhatCanMisleadAReader(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"a newline cannot start a second line": {
			"Archive \"Acme\"\nApproved by security", "Archive \"Acme\" Approved by security",
		},
		"a carriage return cannot overwrite the line": {
			"Archive \"Acme\"\r\r\r  Routine cleanup", "Archive \"Acme\" Routine cleanup",
		},
		"a bidi override cannot reverse what follows": {
			"Send to \u202emoc.rekcatta@ved\u202c", "Send to moc.rekcatta@ved",
		},
		"a directional isolate goes the same way": {
			"Merge \u2066victim\u2069 into \u2067attacker\u2069", "Merge victim into attacker",
		},
		"a zero-width joiner cannot build a homograph": {
			"Archive \"Ac\u200bme\"", "Archive \"Acme\"",
		},
		"ordinary text is left alone": {
			"Send offer to Jörg Müller (€2,500.00)", "Send offer to Jörg Müller (€2,500.00)",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := SanitizeSummary(tc.in); got != tc.want {
				t.Errorf("SanitizeSummary(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A benign prefix long enough to push the real content out of view is the
// cheapest of these attacks and needs no special characters at all.
func TestSanitizeSummaryBoundsALongPrefix(t *testing.T) {
	got := SanitizeSummary(strings.Repeat("routine cleanup ", 200) + "and delete everything")
	if len(got) > maxSummaryLen {
		t.Errorf("summary is %d bytes, want at most %d — the ellipsis is inside the budget", len(got), maxSummaryLen)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a truncated summary must say so, got %q", got[max(0, len(got)-40):])
	}
}

// Bounding must not produce invalid UTF-8. The cut has to land INSIDE a
// multi-byte rune for this to test anything: an ASCII run sized so that the
// budget falls partway through the 日 that follows it puts the naive cut
// between that rune's bytes, and only walking back to the rune start avoids
// emitting half of it.
func TestSanitizeSummaryCutsOnARuneBoundary(t *testing.T) {
	for offset := 1; offset <= 4; offset++ {
		prefix := strings.Repeat("x", maxSummaryLen-offset)
		got := SanitizeSummary(prefix + strings.Repeat("日", 8))
		if strings.ContainsRune(got, '�') {
			t.Errorf("bounding split a rune at offset=%d: %q", offset, got)
		}
		if !utf8.ValidString(got) {
			t.Errorf("bounding produced invalid UTF-8 at offset=%d: %q", offset, got)
		}
		if len(got) > maxSummaryLen {
			t.Errorf("bounded summary is %d bytes at offset=%d, want at most %d", len(got), offset, maxSummaryLen)
		}
	}
}

// The parts reach the same screen as the summary, so each piece is cleaned
// alike. A field whose name cleans away is dropped.
func TestSanitizeSummaryPartsCleansEveryPiece(t *testing.T) {
	got := SanitizeSummaryParts(&crmcontracts.ApprovalSummaryParts{
		Sentence: "Send to \u202emoc.rekcatta@ved\u202c",
		Fields: []crmcontracts.ApprovalSummaryField{
			{Field: "to", Value: "a@b.test\nApproved by security"},
			{Field: "\u200b", Value: "hidden"},
		},
		MoreFields: -3,
	})
	want := crmcontracts.ApprovalSummaryParts{
		Sentence:   "Send to moc.rekcatta@ved",
		Fields:     []crmcontracts.ApprovalSummaryField{{Field: "to", Value: "a@b.test Approved by security"}},
		MoreFields: 1,
	}
	if got == nil || got.Sentence != want.Sentence || got.MoreFields != want.MoreFields || !slices.Equal(got.Fields, want.Fields) {
		t.Errorf("cleaned parts = %+v, want %+v", got, want)
	}
	if SanitizeSummaryParts(nil) != nil {
		t.Error("no parts cleaned into some")
	}
}

// A withheld plan hides the summary in both forms; the parts carry the same
// values the joined line does.
func TestWithholdingThePayloadHidesTheSummaryParts(t *testing.T) {
	summary := "Update record deal: amount=100"
	approval := crmcontracts.Approval{
		Summary:      &summary,
		SummaryParts: &crmcontracts.ApprovalSummaryParts{Sentence: "Update record deal"},
	}
	withholdPayload(&approval)
	if approval.Summary != nil || approval.SummaryParts != nil {
		t.Errorf("withheld approval still shows summary %v or parts %+v", approval.Summary, approval.SummaryParts)
	}
}
