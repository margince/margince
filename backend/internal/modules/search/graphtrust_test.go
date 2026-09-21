// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The trust ladder reads WHO said it, off captured_by.
//
// It used to read activity.source, which stopped meaning "a human typed this"
// the moment agent tool calls began writing source=manual like every other
// first-party write. This test is the pin: it is written against captured_by
// values, so keying the ladder back onto a channel word fails it.
func TestTheTrustLadderReadsTheWriterNotTheChannel(t *testing.T) {
	human := "human:" + ids.NewV7().String()
	agent := "agent:" + ids.NewV7().String()

	for _, c := range []struct {
		name       string
		capturedBy string
		imported   bool
		want       float64
	}{
		{"a human said it", human, false, trustHumanStatement},
		{"an agent wrote it", agent, false, trustAgentWrite},
		{"a connector brought it in", "connector:gmail", false, trustCapturedExternal},
		{"nobody is named", "", false, trustCapturedExternal},
		{"the old channel word, which names no writer", "manual", false, trustCapturedExternal},
		// The import case, and the reason the parameter exists. captured_by
		// names the administrator who ran the import — truthfully, and about
		// the wrong one — so the human prefix here must NOT win.
		{"an import wrote it under an administrator's seat", human, true, trustCapturedExternal},
		{"an import with no writer named at all", "", true, trustCapturedExternal},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := trustOfWriter(c.capturedBy, c.imported); got != c.want {
				t.Errorf("trustOfWriter(%q, imported=%v) = %v, want %v",
					c.capturedBy, c.imported, got, c.want)
			}
		})
	}
}

// TestANativelySentMessageIsNotImported is the regression for the predicate
// this change corrects, and it is a test about the CALLER's question rather
// than about the ladder.
//
// An earlier draft derived `imported` from `source_system IS NOT NULL`. That
// column is not exclusive to imports: activities/outboundmessage.go stamps
// `email` on a message this installation sent, and activities/requesttask.go
// stamps its own reminder identity. Under that predicate both became T2 —
// first-party work ranked as somebody else's captured history.
//
// The namespace is what separates them, so this pins the two spellings apart
// at the only place a Go test can see them: what the prefix test answers.
func TestANativelySentMessageIsNotImported(t *testing.T) {
	for _, c := range []struct {
		name         string
		sourceSystem string
		want         bool
	}{
		{"a message this installation sent", connector.EmailSourceSystem, false},
		{"an internal reminder's own identity", provenance.EmailRequestSource, false},
		{"a row typed here, carrying no source at all", "", false},
		{"an imported row", provenance.ReservedSourceSystemPrefix + "hubspot", true},
		{"the CSV importer's rows", provenance.ReservedSourceSystemPrefix + "csv", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := strings.HasPrefix(c.sourceSystem, provenance.ReservedSourceSystemPrefix)
			if got != c.want {
				t.Errorf("source_system %q reads as imported=%v, want %v — the graph walk asks "+
					"this same question in SQL, so a mismatch here is a mis-ranked timeline",
					c.sourceSystem, got, c.want)
			}
		})
	}
}

// Two notes of the same age rank by who wrote them: the human statement first.
//
// This is the property case 6 rests on — asking what happened with a client
// must surface what a contact recorded ahead of what an agent inferred.
func TestAHumanNoteOutranksAnAgentNoteOfTheSameAge(t *testing.T) {
	now := time.Now()
	when := now.Add(-72 * time.Hour)
	humanScore := rankScore(0, when, "human:"+ids.NewV7().String(), false, now)
	agentScore := rankScore(0, when, "agent:"+ids.NewV7().String(), false, now)
	if humanScore <= agentScore {
		t.Fatalf("the human note scored %v and the agent note %v; the human one must rank higher",
			humanScore, agentScore)
	}
}

// Recency still outweighs trust, which is the §10.7.2 weighting and not an
// accident: a captured email from this morning is more use than a human note
// from last quarter, and the ladder is a tie-break between comparable ages
// rather than an override.
func TestFreshCapturedContentStillBeatsAStaleHumanNote(t *testing.T) {
	now := time.Now()
	fresh := rankScore(0, now, "connector:gmail", false, now)
	stale := rankScore(0, now.Add(-365*24*time.Hour), "human:"+ids.NewV7().String(), false, now)
	if fresh <= stale {
		t.Fatalf("fresh captured content scored %v and a year-old human note %v; recency must still win",
			fresh, stale)
	}
}
