// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// Every reason a standing grant needs renewing must reach the card that asks
// for it.
//
// A rep's overnight grant can stop working in more than one way, and none of
// them fails a run: the runner DEGRADES an under-funded job before its first
// model step, which is right for a misconfiguration and invisible from the
// rep's side. No error, no expiry, no prompt — their overnight work simply
// stops. The settings card is the only thing that can tell them, so a cause the
// server computes and the card never reads is a field that reads as meaningful
// and proves nothing.
//
// THE CORPUS IS THE CONTRACT. MyAgentGrant's `credential_*` booleans are what
// the server says about the credential behind a granted answer, so adding one
// is how a new cause arrives. The card has to branch on each of them. It may
// branch on them TOGETHER — a lapsed passport funds nothing, so the two known
// causes overlap and the card shows the actionable one — but it may not be
// silent about one.
//
// Read from crm.yaml rather than from the generated Go: the field is added to
// the schema first, and a gate reading the generated side would agree with a
// tree nobody had regenerated yet.

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	grantRenewalContract = "api/crm.yaml"
	grantRenewalCard     = "../frontend/src/screens/overnight-grant.tsx"
)

// TestEveryRenewalCauseReachesTheCardThatAsksForIt is the parity check.
func TestEveryRenewalCauseReachesTheCardThatAsksForIt(t *testing.T) {
	t.Parallel()
	causes := credentialFieldsOfMyAgentGrant(t)
	// Two today: usable, and funds_agent. A reader that found none would report
	// a clean sweep over a card that branches on nothing.
	if len(causes) < 2 {
		t.Fatalf("found %d credential field(s) on MyAgentGrant and expects at least 2 — either the schema lost one "+
			"or this reader has stopped matching, and it cannot tell the difference: %v", len(causes), causes)
	}
	card, err := os.ReadFile(grantRenewalCard)
	if err != nil {
		t.Fatalf("reading the settings card: %v", err)
	}
	for _, cause := range causes {
		// The field READ, not the name anywhere in the file. A card that
		// carried the name in a type annotation, a comment or a dead
		// expression would satisfy a substring search while ignoring the cause
		// entirely — the exact silence this gate exists to break.
		if !strings.Contains(string(card), "grant?."+cause) &&
			!strings.Contains(string(card), "grant."+cause) {
			t.Errorf("MyAgentGrant.%s says a rep's standing grant has stopped working, and %s never READS it (no "+
				"`grant.%[1]s`) — "+
				"so the run degrades every night and nothing on the surface the rep can act on says why. Branch on "+
				"it beside the causes already there",
				cause, grantRenewalCard)
		}
	}
}

// credentialFieldsOfMyAgentGrant names the schema's `credential_` booleans.
//
// PARSED, not pattern-matched: rewriting the schema block-style changes nothing
// about the contract, and a regex over the raw file would walk onto a different
// schema's properties and report a confident wrong answer about a list it was
// no longer reading.
func credentialFieldsOfMyAgentGrant(t *testing.T) []string {
	t.Helper()
	document, err := os.ReadFile(grantRenewalContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]struct {
				// Type is `any`: a property elsewhere in this document
				// declares its type as a LIST, and a reader typed to a
				// string fails on the whole file rather than on the one
				// schema it is about.
				Properties map[string]struct {
					Type any `yaml:"type"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(document, &contract); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	schema, declared := contract.Components.Schemas["MyAgentGrant"]
	if !declared {
		t.Fatal("the contract declares no MyAgentGrant schema: this gate reads that schema for the causes a rep " +
			"can be asked to renew, so it now certifies nothing")
	}
	var out []string
	for name, property := range schema.Properties {
		if strings.HasPrefix(name, "credential_") && property.Type == any("boolean") {
			out = append(out, name)
		}
	}
	return out
}
