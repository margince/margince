// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"sort"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const (
	// LanguageEvidenceTurns bounds how many turns a draft reads for its language.
	LanguageEvidenceTurns = 12
	// LanguageEvidenceRunes bounds each evidence string.
	LanguageEvidenceRunes = TurnTextRunes + 200
)

// LanguageEvidence is the text a draft's language is read from, one string per
// message with subject and body together. Inbound mail comes first, newest
// first, because it is the best evidence of the other side's language.
// Our outbound mail and logged notes follow. So a thread where only we wrote
// still resolves.
//
// One string per message rather than a joined blob. textlang.FirstKnown takes
// the first text that clears the detector. So a short message does not dilute
// a clear one next to it.
func LanguageEvidence(turns []Turn) []string {
	ordered := make([]Turn, 0, len(turns))
	for _, turn := range turns {
		if strings.TrimSpace(turn.Subject) != "" || strings.TrimSpace(turn.Text) != "" {
			ordered = append(ordered, turn)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := isInbound(ordered[i]), isInbound(ordered[j])
		if a != b {
			return a
		}
		return ordered[i].At.After(ordered[j].At)
	})
	if len(ordered) > LanguageEvidenceTurns {
		ordered = ordered[:LanguageEvidenceTurns]
	}
	out := make([]string, 0, len(ordered))
	for _, turn := range ordered {
		out = append(out, textcut.Runes(strings.TrimSpace(turn.Subject+"\n"+turn.Text), LanguageEvidenceRunes))
	}
	return out
}

// LanguageEvidenceOf reads a record's recent activities, newest first, through
// the shared conversation reader and returns their language evidence. The
// reader keeps every row so LanguageEvidence orders before it cuts: newer
// outbound mail must not push an older inbound message out of the window.
func LanguageEvidenceOf(activities []crmcontracts.Activity, now time.Time) []string {
	return LanguageEvidence(Conversation(activities, nil, now, len(activities)))
}

func isInbound(turn Turn) bool {
	return turn.Direction == string(crmcontracts.ActivityDirectionInbound)
}
