// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A refusal that names a closed set must deliver the WHOLE set.
//
// The defect these hold against: this renderer bounded a classified fault's
// detail with the figure sized for a bad-args echo, so the set — the half a
// caller acts on — was what the truncation took. A report's block grammar never
// reached a caller at all, and the analytics populations arrived cut mid-name,
// which is worse than absent because a partial list reads as a complete one.
//
// The vocabularies themselves are NOT named here. They live in compose, which
// is downstream of this package, so this file proves the MECHANISM against a
// set of its own and the gate proves the real ones fit.

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func newTestDispatcher() *Dispatcher {
	return NewDispatcher(nil, nil, "t", "0").
		WithLogger(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}

// closedSetRefusal is the shape both real producers take: one detail carrying
// prose that names what was refused and then the whole set that would have
// worked, with NO per-field breakdown. The set is last because that is how a
// reader meets it, and last is what a truncation eats.
//
// No Fields, deliberately — that is the branch the report validator and the
// analytics schema actually reach, and a fault carrying one is rendered by the
// other half of faultExplanation on a different budget.
func closedSetRefusal(detail string) error {
	return &httperr.DetailedError{
		Status: http.StatusBadRequest,
		Code:   "invalid_argument",
		Detail: detail,
	}
}

// blockKindRefusal is the report validator's message, as reportdoc spells it.
func blockKindRefusal(members []string) string {
	return `block 0 ("paragraph"): no such block kind. A renderer meeting one either draws ` +
		`something nobody specified or drops it silently, and a report missing a block it ` +
		`was composed with says something different from the one composed. The kinds are: ` +
		strings.Join(members, ", ")
}

func TestARefusalDeliversTheWholeClosedSet(t *testing.T) {
	members := []string{
		"title", "subtitle", "scope", "generated_at", "summary", "methodology",
		"follow_ups", "stat_strip", "bar", "waterfall", "ranked_list",
		"record_table", "callout", "evidence_drawer",
	}

	got := newTestDispatcher().explain("compose_analytics_report", closedSetRefusal(blockKindRefusal(members)))

	for _, member := range members {
		if !strings.Contains(got, member) {
			t.Errorf("the refusal never names %q, so a caller cannot pick it:\n%s", member, got)
		}
	}
	if strings.Contains(got, "…") {
		t.Errorf("the refusal was truncated, and the set is what truncation takes:\n%s", got)
	}
}

// The bound still bounds. Raising it for our own text must not turn it into no
// ceiling at all: the detail lands in a transcript whose later prompts the same
// model reads, so an unbounded one is an unbounded write into every one of them.
func TestAnOverlongDetailIsStillCut(t *testing.T) {
	got := newTestDispatcher().explain("compose_analytics_report",
		closedSetRefusal(strings.Repeat("x", MaxFaultDetail+64)))

	if !strings.Contains(got, "…") {
		t.Errorf("a detail past the ceiling was not cut:\n%s", got)
	}
	if strings.Count(got, "x") > MaxFaultDetail {
		t.Errorf("the cut admitted %d bytes of detail, past the %d ceiling",
			strings.Count(got, "x"), MaxFaultDetail)
	}
}

// The bound is a CEILING and not a target: our own prose is admitted whole up
// to it, so a refusal shorter than the ceiling is never padded or cut.
//
// That the two real producers bound the caller's token where it enters — which
// is what keeps the ceiling spent on the vocabulary rather than on whatever was
// sent — is held where those producers are visible, in the gate.
func TestARefusalUnderTheCeilingIsUntouched(t *testing.T) {
	detail := blockKindRefusal([]string{"title", "summary", "callout"})

	got := newTestDispatcher().explain("compose_analytics_report", closedSetRefusal(detail))

	if !strings.Contains(got, detail) {
		t.Errorf("a refusal inside the ceiling did not travel verbatim:\nwant substring: %s\ngot: %s", detail, got)
	}
}
