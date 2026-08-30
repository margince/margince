// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The shape a corpus fixture's trigger ref must have, DERIVED from the writer
// that mints one in production rather than restated here.
//
// The agent_loop corpus is production-shaped on purpose: its whole value is
// certifying the window the runner actually builds. A fixture whose trigger ref
// drifts from what the scheduler puts on a job certifies a window nothing
// builds, and there is no failing assertion when that happens — the suite
// simply reports PASS about a different system.

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// occurrenceTriggerKinds are the trigger refs of the form `<kind>:<uuid>` this
// corpus exercises. They are DECLARED rather than derived, because no
// production writer mints one yet: the scheduler's specs are the only trigger
// source this build has, and the corpus reaches ahead of it on purpose — a
// `calendar:<uuid>` is the confusable shape window.go's triggerProvenance
// sentence exists to answer, and certifying it before the writer exists is the
// point of having it here.
//
// Whoever adds that writer should delete this map and derive its arm the way
// the scheduled arm below is derived. Until then this list is the seam, and
// the reason it is short is that reaching ahead is only honest for a shape
// somebody has actually argued for.
var occurrenceTriggerKinds = map[string]string{
	"manual":   "a run a person started by hand, which the corpus drives because a hand-started turn is the same window with a different name on it",
	"calendar": "the occurrence-driven shape triggerProvenance names as the confusable one; certifying it before its writer exists is why the corpus carries it",
}

// refuseUnmintableTriggerRef names a trigger ref no production path could have
// put on a job.
//
// The whole value of this corpus is that it is production-shaped: it certifies
// the window the runner actually builds. A non-empty check was all this used to
// do, so a fixture could drift arbitrarily far from production and stay
// accepted — and one did. When the ref grew a seat segment
// (`<spec>:<date>` → `<spec>:<date>:<seat digest>`) a fixture kept the old
// two-segment value and every gate stayed green, because there is no failing
// assertion when a fixture merely describes a different system. A reviewer
// reading the diff found it.
//
// So the scheduled arm is DERIVED from the writer. AgentSpec.TriggerRef is
// minted here with a known day and seat, and the fixture is required to have
// the same segment count and the same per-segment shape as what came back.
// Nothing in this function restates the format, so the day TriggerRef grows a
// fourth segment or moves the date, every stale fixture fails naming itself.
func refuseUnmintableTriggerRef(ref string) error {
	segments := strings.Split(ref, ":")
	kind := segments[0]
	if reason, occurrence := occurrenceTriggerKinds[kind]; occurrence {
		if parsed, err := ids.Parse(segments[1]); len(segments) != 2 || err != nil || parsed.IsZero() {
			return fmt.Errorf(
				"%s: trigger ref %q names the occurrence-driven kind %q (%s), whose shape is `%s:<uuid>` — this one is not",
				agentLoopSite, ref, kind, reason, kind)
		}
		return nil
	}
	reference, ok := mintedSchedulerTriggerRef(kind)
	if !ok {
		return fmt.Errorf(
			"%s: trigger ref %q starts with %q, which is neither a spec in the agent catalog nor one of the "+
				"occurrence-driven kinds this corpus reaches ahead for (%s) — a fixture naming a trigger no writer "+
				"mints certifies a window the product never builds",
			agentLoopSite, ref, kind, strings.Join(sortedKeys(occurrenceTriggerKinds), ", "))
	}
	return refuseDriftedFrom(reference, ref)
}

// mintedSchedulerTriggerRef mints what the scheduler would put on a job for the
// named spec, using a fixed day and seat so the value is the SHAPE and nothing
// else. It reports false for a name no catalog spec carries.
func mintedSchedulerTriggerRef(specName string) (string, bool) {
	for _, spec := range runner.Catalog() {
		if spec.Name != specName {
			continue
		}
		return spec.TriggerRef(referenceTriggerDay, referenceTriggerSeat), true
	}
	return "", false
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
// without re-spelling the format here, and it is enough — the drift that shipped
// was a missing segment, and a digest of the wrong width or a date that is not a
// date fails the same way.
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
		if len(got[i]) != len(want[i]) || charactersOf(got[i]) != charactersOf(want[i]) {
			return fmt.Errorf(
				"%s: trigger ref %q segment %d is %q; the scheduler mints segments shaped like %q here (%q) — the "+
					"fixture has drifted from what production puts on a job",
				agentLoopSite, ref, i+1, got[i], want[i], minted)
		}
	}
	return nil
}

// charactersOf is the character classes a segment is drawn from, in a settled
// order. It is deliberately coarse: it separates a date from a digest and a
// digest from a word, and it does NOT try to be a format, because a format
// stated here is the second spelling this whole function exists to avoid.
func charactersOf(segment string) string {
	var classes []string
	for _, class := range []struct {
		name string
		has  func(rune) bool
	}{
		{"digit", func(r rune) bool { return r >= '0' && r <= '9' }},
		{"hex-letter", func(r rune) bool { return r >= 'a' && r <= 'f' }},
		{"letter", func(r rune) bool { return (r >= 'g' && r <= 'z') || (r >= 'A' && r <= 'Z') }},
		{"dash", func(r rune) bool { return r == '-' || r == '_' }},
	} {
		if strings.ContainsFunc(segment, class.has) {
			classes = append(classes, class.name)
		}
	}
	return strings.Join(classes, "+")
}
