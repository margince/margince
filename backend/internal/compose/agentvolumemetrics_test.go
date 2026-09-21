// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
)

// The scrape an operator alerts on. A role whose bound cannot reach its
// counter store is refusing every agent read; the only way to see that on a
// split role is this line, because /readyz deliberately does not probe it.
func TestAnUnreachableBoundScrapesAsUnanswerable(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	// No Redis: the fail-closed composition, which is what an unreachable
	// store leaves the meter in.
	Server{volumeMeter: agentvolume.New(nil, agentvolume.Limits{}, time.Minute)}.writeAgentVolumeSection(&out)

	scrape := out.String()
	if !strings.Contains(scrape, "margince_agent_volume_bound 1") {
		t.Errorf("a role that composed a bound scrapes:\n%s\nwant it reporting one", scrape)
	}
	if !strings.Contains(scrape, "margince_agent_volume_answerable 0") {
		t.Errorf("a bound that cannot reach its store scrapes:\n%s\nwant it reporting itself unanswerable", scrape)
	}
	// The sentence that stops an operator draining pods over an agent-only
	// fault has to be ON the metric, where an alert rule quotes it from.
	if !strings.Contains(scrape, "Human traffic is unaffected") {
		t.Errorf("the HELP text does not say human traffic is unaffected:\n%s", scrape)
	}
}

// A role that declared it serves no bounded agent surface must not look broken
// for ever — which is what one "healthy" gauge would have made it.
func TestAnUnboundedRoleScrapesAsUnboundRatherThanFailing(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	Server{volumeMeter: agentvolume.Unmetered()}.writeAgentVolumeSection(&out)

	if scrape := out.String(); !strings.Contains(scrape, "margince_agent_volume_bound 0") {
		t.Errorf("an unmetered role scrapes:\n%s\nwant it reporting no bound", scrape)
	}
}

// Every role renders the family, including one that wired no meter: an absent
// series is indistinguishable from a scrape nobody is collecting, and this is
// the metric whose whole job is to be noticed when it changes.
func TestTheFamilyIsRenderedEvenByARoleThatWiredNoMeter(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	Server{}.writeAgentVolumeSection(&out)

	for _, want := range []string{"margince_agent_volume_bound 0", "margince_agent_volume_answerable 0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("a role with no meter scrapes:\n%s\nwant %q", out.String(), want)
		}
	}
}
