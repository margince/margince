// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsummary

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBriefAndFullReadTheLongMessagesFinalConditionsAndEarlierCommitment(t *testing.T) {
	tail := "Berlin only. Security requests 12 June, but Jules has not accepted it. Backup deletion has no due date."
	earlier := "Jules accepted 14 June, conditional on the approved policy."
	for _, brief := range []bool{false, true} {
		in := Input{Brief: brief, Record: "company Pilot", Earlier: strings.Repeat("Earlier discussion. ", 100) + earlier, Messages: []Message{{Author: "Alex", Side: SideColleague, Text: strings.Repeat("Implementation background. ", 150) + tail}}}
		request, err := Request(in, nil)
		if err != nil {
			t.Fatal(err)
		}
		prompt := request.Messages[0].Content
		if !strings.Contains(prompt, tail) || !strings.Contains(prompt, earlier) {
			t.Fatal("a long source lost its tail or its earlier accepted commitment")
		}
		for _, rule := range []string{"Distinguish a requested date from an accepted commitment", "Keep an unaccepted change open for reconfirmation", "Never infer a deadline for an unanswered question"} {
			if !strings.Contains(request.System, rule) {
				t.Fatalf("missing summary rule: %s", rule)
			}
		}
	}
}

func TestAnOversizedSourceIsRefusedInsteadOfBeingSummarisedInPart(t *testing.T) {
	for _, in := range []Input{
		{Messages: make([]Message, maxMessages+1)},
		{Messages: []Message{{Text: strings.Repeat("x", maxInputRunes+1)}}},
		{Earlier: strings.Repeat("x", maxInputRunes+1), Messages: []Message{{Text: "A later request"}}},
		{Messages: []Message{{Text: "A source", Side: Side(-1)}}},
	} {
		request, err := Request(in, nil)
		if !errors.Is(err, ErrIncompleteInput) || len(request.Messages) != 0 {
			t.Fatalf("incomplete source produced a model request: %v", err)
		}
	}
}

func TestBriefDetailLimitsTheWordingWithoutShorteningTheModelInput(t *testing.T) {
	reply := Reply{Relevant: true, Title: "Policy review", Summary: strings.Repeat("x", BriefSummaryRunes+1)}
	if err := CheckForInput(reply, Input{Brief: true}); err == nil {
		t.Fatal("brief response ignored its display bound")
	}
	if err := CheckForInput(reply, Input{}); err != nil {
		t.Fatalf("full response inherited the brief bound: %v", err)
	}
}

func TestAnOverlongDecisionKeepsItsConditionAndIsRejectedRatherThanCut(t *testing.T) {
	decision := strings.Repeat("approved with conditions; ", 20) + "only after the customer signs"
	raw, err := json.Marshal(Reply{Relevant: true, Title: "Conditional approval", Summary: "The approval still requires a signature.", Decisions: []string{decision}})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Decisions) != 1 || reply.Decisions[0] != decision {
		t.Fatal("parsing removed the final condition")
	}
	if err := Check(reply); err == nil {
		t.Fatal("overlong decision was accepted without rewording")
	}
}

func TestReconfirmationMustQuoteOneAllowedMessageAndCannotInventADeadline(t *testing.T) {
	quote := "Security requests 12 June, but Jules has not accepted it."
	reply := Reply{Relevant: true, Title: "Date needs reconfirmation", Summary: "Jules accepted 14 June. The earlier request remains unaccepted.", Reconfirmation: &Reconfirmation{Question: "Can Jules confirm whether the requested change is possible?", Quote: quote}}
	if err := CheckForInput(reply, Input{Messages: []Message{{Text: quote}}}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Different source", quote[:20]} {
		if err := CheckForInput(reply, Input{Messages: []Message{{Text: text}}}); err == nil {
			t.Fatal("accepted an ungrounded reconfirmation")
		}
	}
	reply.Reconfirmation.Quote = strings.Repeat("x", 501)
	if err := CheckForInput(reply, Input{Messages: []Message{{Text: reply.Reconfirmation.Quote}}}); err == nil {
		t.Fatal("accepted an oversized evidence quote")
	}
}
