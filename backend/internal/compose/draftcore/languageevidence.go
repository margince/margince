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
// first, because it is the best evidence of the other side's language. The
// addressee's own mail leads it, since another participant may write in a
// different language. An empty addressee ranks all inbound mail alike.
// Our outbound mail and logged notes follow. So a thread where only we wrote
// still resolves.
//
// One string per message rather than a joined blob. textlang.FirstKnown takes
// the first text that clears the detector. So a short message does not dilute
// a clear one next to it.
func LanguageEvidence(turns []Turn, addressee string) []string {
	ordered := make([]Turn, 0, len(turns))
	for _, turn := range turns {
		if strings.TrimSpace(turn.Subject) != "" || strings.TrimSpace(turn.Text) != "" {
			ordered = append(ordered, turn)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := evidenceRank(ordered[i], addressee), evidenceRank(ordered[j], addressee)
		if a != b {
			return a < b
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

// AddresseeEvidence is the text of the addressee's own inbound mail, newest
// first and bounded like LanguageEvidence. It carries no subject: a reply often
// keeps the subject line our side wrote, in our language. A logged call or
// meeting is a rep's words, so only mail and messages count.
func AddresseeEvidence(turns []Turn, addressee string) []string {
	var own []Turn
	for _, turn := range turns {
		written := turn.Kind == crmcontracts.ActivityKindEmail || turn.Kind == crmcontracts.ActivityKindMessage
		if written && addressee != "" && evidenceRank(turn, addressee) == 0 && strings.TrimSpace(turn.Text) != "" {
			own = append(own, turn)
		}
	}
	sort.SliceStable(own, func(i, j int) bool { return own[i].At.After(own[j].At) })
	out := make([]string, 0, min(len(own), LanguageEvidenceTurns))
	for _, turn := range own[:min(len(own), LanguageEvidenceTurns)] {
		out = append(out, textcut.Runes(strings.TrimSpace(turn.Text), LanguageEvidenceRunes))
	}
	return out
}

// LanguageEvidenceOf reads a record's recent activities, newest first, through
// the shared conversation reader and returns their language evidence. The
// reader keeps every row so LanguageEvidence orders before it cuts: newer
// outbound mail must not push an older inbound message out of the window.
func LanguageEvidenceOf(activities []crmcontracts.Activity, now time.Time, zone *time.Location) []string {
	return LanguageEvidence(Conversation(activities, nil, now, zone, len(activities)), "")
}

// evidenceRank orders evidence: the addressee's mail, other mail they wrote,
// then everything else. Who wrote it, never which way it went.
//
// A colleague writing from one of our own domains arrives inbound. Ranked by
// direction, their mail was a sample of the counterparty's language.
func evidenceRank(turn Turn, addressee string) int {
	switch {
	case !turn.TheirWord():
		return 2
	case addressee != "" && turn.SenderID == addressee:
		return 0
	default:
		return 1
	}
}
