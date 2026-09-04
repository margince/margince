// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The block grammar has two doors and one composition.

import (
	"context"
	"strings"
	"testing"
)

func aGrammar() BlockGrammar {
	return BlockGrammar{
		Blocks: []BlockDescription{
			{Kind: "title", TakesText: true},
			{Kind: "stat_strip", TakesCells: true},
			{Kind: "callout", TakesText: true, TakesSeverity: true},
		},
		Severities: []string{"note", "warning"},
	}
}

// The resource and the tool seam serve the SAME bytes.
//
// Two doors onto one document is the point of publishing it at all: a caller
// that reads resources and a scheduled agent that cannot must be told the same
// grammar. Two renderings would agree today and drift the first time one grew a
// field.
func TestTheBlocksResourceAndTheSeamServeTheSameBytes(t *testing.T) {
	r := NewReportBlocksResource(aGrammar())
	ctx := context.Background()

	viaSeam, err := r.ReportBlocksDocument(ctx)
	if err != nil {
		t.Fatalf("composing through the seam: %v", err)
	}
	viaResource, err := r.ReadResource(ctx, ReportBlocksURI)
	if err != nil {
		t.Fatalf("reading through the resource: %v", err)
	}
	if viaResource.Text != string(viaSeam) {
		t.Errorf("the two doors serve different bytes.\nresource: %s\nseam:     %s",
			viaResource.Text, viaSeam)
	}
}

// The document carries the grammar it was handed, and the rule a field list
// cannot carry.
func TestTheBlocksDocumentCarriesTheGrammarAndTheRule(t *testing.T) {
	body, err := NewReportBlocksResource(aGrammar()).ReportBlocksDocument(context.Background())
	if err != nil {
		t.Fatalf("composing the document: %v", err)
	}
	for _, want := range []string{"title", "stat_strip", "callout", "note", "warning"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the document does not carry %q: %s", want, body)
		}
	}
	// The literal rule travels with the grammar, because it is the one thing a
	// composer gets wrong that no list of kinds and fields would tell them.
	if !strings.Contains(string(body), "EVEN WHEN") {
		t.Error("the document does not carry the rule that a literal is refused beside a " +
			"valid citation — the one thing a field list cannot say")
	}
}

// An unknown URI reads as not-found, matching every other read on this surface.
func TestTheBlocksResourceRefusesAnotherURI(t *testing.T) {
	if _, err := NewReportBlocksResource(aGrammar()).
		ReadResource(context.Background(), "margince://schema/reports"); err == nil {
		t.Error("the block grammar answered for another document's URI")
	}
}
