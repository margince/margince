// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

//go:build !integration

package gates

// Every consumer of a stream an import writes has decided what it does with an
// imported event. A group nobody classified would act on imported events as
// news, starting automations and accruing commissions, with nothing failing.
// The decision lives in events.ImportedDecisionOf; the subscriber enforces it.
// Subjects come from events.Groups(), events.ImportedStreams() and
// public-events.yaml.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/pkg/extension"
)

// groupReadsAnImportedStream reports whether a group's streams include one an
// import writes on.
func groupReadsAnImportedStream(group events.Group) bool {
	imported := events.ImportedStreams()
	return slices.ContainsFunc(group.Streams, func(stream string) bool { return imported[stream] })
}

func TestEveryConsumerOfDealAndActivityEventsDecidesAboutImportedOnes(t *testing.T) {
	t.Parallel()
	readers := map[string]bool{}
	for _, group := range events.Groups() {
		if !groupReadsAnImportedStream(group) {
			continue
		}
		readers[group.Name] = true
		decision, decided := events.ImportedDecisionOf(group.Name)
		if !decided || decision.Reason == "" {
			t.Errorf("%s reads a stream imports write on and has no decision with a reason in "+
				"events.importedDecisions: say whether it skips or keeps an imported event, and why", group.Name)
		}
	}
	// A census that found no reader reads like a tree where every reader is
	// decided.
	const wantAtLeast = 10
	if len(readers) < wantAtLeast {
		t.Fatalf("found %d group(s) reading the deal or activity stream, want at least %d: the derivation lost its subjects",
			len(readers), wantAtLeast)
	}
	for _, name := range events.ImportedDecisionGroups() {
		if !readers[name] {
			t.Errorf("events.importedDecisions names %q, which is no group that reads a stream imports write on: "+
				"remove the entry, or fix the group's name", name)
		}
	}
}

func TestAnExtensionListenerLeavesImportedEventsOut(t *testing.T) {
	t.Parallel()
	name := compose.ComposedSubscription{Unit: "demo", Sub: extension.Subscription{Name: "react"}}.GroupName()
	if !events.SkipsImported(name) {
		t.Fatalf("the extension group %q does not skip imported events: a unit would act on imported history as news", name)
	}
}

// TestEveryStreamTheImportMarkerRidesIsOneThatConsumersAreDecidedFor reads the
// contract: a payload schema with import_run_id puts imported events on its
// event type's stream.
func TestEveryStreamTheImportMarkerRidesIsOneThatConsumersAreDecidedFor(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "backend", publicEventsPath))
	if err != nil {
		t.Fatalf("reading the public event contract: %v", err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]struct {
				EventType  string                    `yaml:"x-event-type"`
				Properties map[string]map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("parsing the public event contract: %v", err)
	}
	marked := 0
	for name, schema := range contract.Components.Schemas {
		if _, ok := schema.Properties["import_run_id"]; !ok {
			continue
		}
		marked++
		stream, err := events.StreamFor(schema.EventType)
		if err != nil {
			t.Errorf("%s carries import_run_id and names the event type %q, which has no stream: %v", name, schema.EventType, err)
			continue
		}
		if !events.ImportedStreams()[stream] {
			t.Errorf("%s carries import_run_id and rides %s, which events.ImportedStreams() does not name: "+
				"its consumers have no decision about imported events", name, stream)
		}
	}
	if marked == 0 {
		t.Fatal("found no payload schema carrying import_run_id: the contract read lost its subjects")
	}
}
