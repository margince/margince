// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// inputEventCensus is every catalog type that can change what the input check
// read: the deal stream, which carries deals, offers, contracts, stages and
// rooms, and the forecast family.
func inputEventCensus(t *testing.T, catalog []string, route func(string) (string, error)) []string {
	t.Helper()
	dealStream := routed(t, events.StreamFor, "deal.updated")
	var out []string
	for _, eventType := range catalog {
		stream := routed(t, route, eventType)
		if stream == dealStream || strings.HasPrefix(eventType, "forecast.") {
			out = append(out, eventType)
		}
	}
	return out
}

func routed(t *testing.T, route func(string) (string, error), eventType string) string {
	t.Helper()
	stream, err := route(eventType)
	if err != nil {
		t.Fatalf("routing %s: %v", eventType, err)
	}
	return stream
}

// undecided is the census types inputChanges has no decision for.
func undecided(census []string) []string {
	var out []string
	for _, eventType := range census {
		if _, ok := InputChangeOf(eventType); !ok {
			out = append(out, eventType)
		}
	}
	return out
}

func TestEveryDealAndForecastEventHasAnInputDecision(t *testing.T) {
	census := inputEventCensus(t, events.Types(), events.StreamFor)
	if len(census) < 40 {
		t.Fatalf("the census found %d deal and forecast types; the catalog holds more, so the filter is wrong", len(census))
	}
	for _, eventType := range undecided(census) {
		t.Errorf("%s rides the deal stream or is a forecast event, and inputChanges has no decision for it: "+
			"say whether it makes the input check's reading out of date", eventType)
	}
	for _, eventType := range DecidedInputEvents() {
		if !slices.Contains(census, eventType) {
			t.Errorf("inputChanges decides %s, which is not a deal-stream or forecast catalog type", eventType)
		}
		if change, _ := InputChangeOf(eventType); change.Why == "" {
			t.Errorf("inputChanges decides %s with no reason", eventType)
		}
	}
}

// A new type on the deal stream is caught, not read past.
func TestANewDealEventWithNoDecisionIsReported(t *testing.T) {
	withNewType := func(eventType string) (string, error) {
		if eventType == "deal.reopened" {
			return events.StreamFor("deal.updated")
		}
		return events.StreamFor(eventType)
	}
	census := inputEventCensus(t, append(events.Types(), "deal.reopened"), withNewType)
	if got := undecided(census); !slices.Equal(got, []string{"deal.reopened"}) {
		t.Fatalf("undecided = %v, want the new type alone", got)
	}
}

// The consumer group must listen where every type that marks anything rides.
func TestTheFreshnessGroupHearsEveryMarkingEvent(t *testing.T) {
	var streams []string
	for _, g := range events.Groups() {
		if g.Name == "cg:assurance-freshness" {
			streams = g.Streams
		}
	}
	if streams == nil {
		t.Fatal("no cg:assurance-freshness group, so nothing marks a reading out of date")
	}
	for _, eventType := range DecidedInputEvents() {
		if change, _ := InputChangeOf(eventType); change.Scope == ScopeNone {
			continue
		}
		if stream := routed(t, events.StreamFor, eventType); !slices.Contains(streams, stream) {
			t.Errorf("%s marks a reading out of date but rides %s, which cg:assurance-freshness does not read", eventType, stream)
		}
	}
}
