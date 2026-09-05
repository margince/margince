// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The queue's verdict standings ARE the deal card's, and this derives them from
// the card rather than keeping a second list of them.
//
// Two schemas in one contract name the same four words. `DealStatusCardVerdict`
// is where a deal's standing is DECIDED — compose/dealstatus writes it and the
// deal page prints it. `WorklistDealVerdict` is where the queue CARRIES that
// answer onto a row. A word on one side only is a real defect with no failing
// assertion of its own: a fifth standing added to the card reaches the queue's
// mapper, is refused as unrecognised, and every row about such a deal quietly
// loses its verdict while every test stays green.
//
// The card's side is deliberately not an enum — its own description says so, and
// names DealRoomParticipantCapability as the precedent — so the four words live
// in its PROSE, one per line, each written as a backticked word followed by an
// em dash. That is the shape this reads.
//
// A GATE THAT HARD-CODED THE FOUR WOULD BE THE SECOND COPY it exists to prevent
// (AGENTS.md, "Reuse before you build" rule 5). So neither list is written here:
// one side is parsed out of the card's description, the other out of the queue's
// enum, and the test compares them. What IS written down is the shape of the
// prose, which is the one thing a parser cannot derive — and the second test
// guards exactly that, by failing when the parse finds too few to be credible
// rather than reporting a happy empty intersection.

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTheQueueCarriesEveryStandingTheDealCardCanDecide(t *testing.T) {
	t.Parallel()
	decided := standingsTheCardDecides(t)
	carried := standingsTheQueueCarries(t)

	if !slices.Equal(decided, carried) {
		t.Errorf("the deal card decides %v and the worklist row carries %v.\n"+
			"align: backend/api/crm.yaml — WorklistDealVerdict.standing must hold exactly the words "+
			"DealStatusCardVerdict.standing describes. A word the card decides and the row does not carry "+
			"is dropped by attention/dealstanding.go's knownStanding, so every row about such a deal loses "+
			"its verdict silently; a word the row carries and the card never decides is a value nothing writes.",
			decided, carried)
	}
}

// The census must not be able to pass by reading nothing. A description reworded
// so the parse finds none would leave both sides empty and equal, which is the
// under-recognition AGENTS.md rule 8 names: it reads a smaller subject, reports
// PASS, and there is no failing assertion to notice.
func TestTheStandingParseFindsTheWordsItIsSupposedTo(t *testing.T) {
	t.Parallel()
	decided := standingsTheCardDecides(t)

	if len(decided) < 4 {
		t.Fatalf("parsed %d standings out of DealStatusCardVerdict.standing (%v), want at least the four "+
			"the product ships. The description's shape changed: this reads lines of the form "+
			"\"`word` — what it means\". Fix the parse here rather than letting the census pass on an empty set.",
			len(decided), decided)
	}
}

// standingWord matches one described standing: a backticked word opening a line,
// followed by the em dash that introduces what it means. Anchored to the line
// start so a backticked word used mid-sentence — the description cites
// DealRoomParticipantCapability that way — is not mistaken for a fifth value.
var standingWord = regexp.MustCompile("(?m)^\\s*`([a-z_]+)`\\s+—")

// standingsTheCardDecides reads the four words out of the deal card verdict's
// prose, where they live because that property is deliberately a plain string.
func standingsTheCardDecides(t *testing.T) []string {
	t.Helper()
	described := crmSchemaProperty(t, "DealStatusCardVerdict", "standing").Description
	found := []string{}
	for _, match := range standingWord.FindAllStringSubmatch(described, -1) {
		found = append(found, match[1])
	}
	sort.Strings(found)
	return slices.Compact(found)
}

// standingsTheQueueCarries reads the enum the worklist row declares.
func standingsTheQueueCarries(t *testing.T) []string {
	t.Helper()
	carried := slices.Clone(crmSchemaProperty(t, "WorklistDealVerdict", "standing").Enum)
	sort.Strings(carried)
	return carried
}

// crmSchemaProperty answers one property of one schema in the public contract.
type crmProperty struct {
	Description string   `yaml:"description"`
	Enum        []string `yaml:"enum"`
}

func crmSchemaProperty(t *testing.T, schema, property string) crmProperty {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]crmProperty `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	found, ok := doc.Components.Schemas[schema]
	if !ok {
		t.Fatalf("the contract declares no schema %s", schema)
	}
	prop, ok := found.Properties[property]
	if !ok {
		t.Fatalf("%s declares no property %s", schema, property)
	}
	return prop
}
