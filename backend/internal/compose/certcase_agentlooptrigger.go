// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The shape a fixture's trigger ref must have, DERIVED from the writer that
// mints one in production rather than restated here.
//
// The agent_loop corpus certifies only the window a scheduled run is handed.
// The scheduler is the only writer of a runner job, so a trigger ref it could
// not have minted is a window nothing builds — and no assertion fails when a
// fixture drifts to one: the suite reports PASS about a different system.

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refuseUnmintableTriggerRef names a trigger ref the scheduler would not put on
// a job of this agent: one naming another agent, or one whose shape has drifted.
//
// The shape is DERIVED. AgentSpec.TriggerRef is minted with a known day and
// seat, and the fixture must have the same segment count and per-segment shape
// as what came back, so the day TriggerRef grows a segment or moves the date,
// every stale fixture fails naming itself.
func refuseUnmintableTriggerRef(agent runner.AgentSpec, ref string) error {
	if kind, _, _ := strings.Cut(ref, ":"); kind != agent.Name {
		return fmt.Errorf(
			"%s/%s: trigger ref %q names %q — the scheduler is the only writer of a runner job, and it names "+
				"every occurrence after the agent that runs it", agentLoopSite, agent.Name, ref, kind)
	}
	return refuseDriftedFrom(agent.TriggerRef(referenceTriggerDay, referenceTriggerSeat), ref)
}

// referenceTriggerDay and referenceTriggerSeat are arbitrary, and the point is
// that they are: what is compared is the shape TriggerRef gives them, never
// these values.
var (
	referenceTriggerDay  = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	referenceTriggerSeat = ids.From[ids.UserKind](ids.MustParse("0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e00"))
)

// refuseDriftedFrom compares a fixture's ref against one production just minted,
// segment by segment: the same count, and each segment the same length and drawn
// from the same characters as the minted one's. That is as much as can be said
// without re-spelling the format here, and it is enough: a missing segment, a
// digest of the wrong width and a date that is not a date all fail on it.
func refuseDriftedFrom(minted, ref string) error {
	want, got := strings.Split(minted, ":"), strings.Split(ref, ":")
	if len(want) != len(got) {
		return fmt.Errorf(
			"%s: trigger ref %q has %d segment(s); the scheduler mints %d for this spec (%q) — a fixture on the old "+
				"shape certifies a window nothing builds",
			agentLoopSite, ref, len(got), len(want), minted)
	}
	// Segment 0 is the spec name and was matched to find `minted` at all.
	for i := 1; i < len(want); i++ {
		mintedClasses, _ := charactersOf(want[i])
		fixtureClasses, readable := charactersOf(got[i])
		if len(got[i]) != len(want[i]) || !readable || fixtureClasses != mintedClasses {
			return fmt.Errorf(
				"%s: trigger ref %q segment %d is %q; the scheduler mints segments shaped like %q here (%q) — the "+
					"fixture has drifted from what production puts on a job",
				agentLoopSite, ref, i+1, got[i], want[i], minted)
		}
	}
	return nil
}

// charactersOf is the character classes a segment is drawn from, in a settled
// order, and whether EVERY rune of it fell into one.
//
// It is deliberately coarse: it separates a date from a digest and a digest
// from a word, and it does NOT try to be a format, because a format stated here
// is the second spelling this whole function exists to avoid.
//
// The second return is what stops the coarseness from being a hole. Presence
// alone would let `2026-01-0!` pass for a date: same width, same classes
// present, one rune that is neither. A segment carrying a rune no class claims
// is not the shape the writer mints, whatever else it looks like.
func charactersOf(segment string) (classes string, readable bool) {
	named := []struct {
		name string
		has  func(rune) bool
	}{
		{"digit", func(r rune) bool { return r >= '0' && r <= '9' }},
		{"hex-letter", func(r rune) bool { return r >= 'a' && r <= 'f' }},
		{"letter", func(r rune) bool { return (r >= 'g' && r <= 'z') || (r >= 'A' && r <= 'Z') }},
		{"dash", func(r rune) bool { return r == '-' || r == '_' }},
	}
	var present []string
	for _, class := range named {
		if strings.ContainsFunc(segment, class.has) {
			present = append(present, class.name)
		}
	}
	for _, r := range segment {
		claimed := false
		for _, class := range named {
			claimed = claimed || class.has(r)
		}
		if !claimed {
			return strings.Join(present, "+"), false
		}
	}
	return strings.Join(present, "+"), true
}
