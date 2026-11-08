// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"errors"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// statusRefs names the records a summary shows: the deal d1, timeline
// activities a1, a2, …, open tasks t1, … and room threads r1, …. Each follows
// the order the summary lists them. StatusRequest and ParseStatus both build it from the
// same input. So a ref reads back to the record it was shown for.
type statusRefs struct {
	deal, acts, tasks, threads *promptref.Refs
}

func refsOf(in StatusInput) statusRefs {
	refs := statusRefs{
		deal: promptref.New("d", in.Deal.ID), acts: promptref.New("a"),
		tasks: promptref.New("t"), threads: promptref.New("r"),
	}
	for _, act := range in.Timeline {
		refs.acts.For(act.ID)
	}
	// A request state or the owed mail can sit past the timeline's window.
	for _, id := range slices.Sorted(maps.Keys(in.RequestStates)) {
		refs.acts.For(id)
	}
	if in.ReplyTo != "" {
		refs.acts.For(in.ReplyTo)
	}
	for _, task := range in.OpenTasks {
		refs.tasks.For(task.ID)
	}
	if in.Room != nil {
		for _, thread := range in.Room.Threads {
			refs.threads.For(thread.ID)
		}
	}
	return refs
}

func (r statusRefs) tables() []*promptref.Refs {
	return []*promptref.Refs{r.deal, r.acts, r.tasks, r.threads}
}

// recordID is the id a cited ref names. It is "" for a ref this summary never
// showed (a bare record id included), which the grounding filter then drops.
func (r statusRefs) recordID(ref string) string {
	for _, table := range r.tables() {
		if id, ok := table.ID(ref); ok {
			return id
		}
	}
	return ""
}

var errRefInReaderText = errors.New("reply spells a record reference in reader text")

// read turns every cited ref of the reply into the record id it names. It
// refuses a reply whose reader text spells a ref, as it refuses one spelling
// an id.
func (r statusRefs) read(parsed replyShape) (replyShape, error) {
	for _, lines := range []replyLines{parsed.Story, parsed.Blocker, parsed.Buyer, parsed.Verdict.Because, parsed.MoveReason} {
		for i := range lines {
			for _, table := range r.tables() {
				if _, named := table.NamedIn(lines[i].Text); named {
					return replyShape{}, errRefInReaderText
				}
			}
			cited := make([]string, len(lines[i].Evidence))
			for j, ref := range lines[i].Evidence {
				cited[j] = r.recordID(ref)
			}
			lines[i].Evidence = cited
		}
	}
	return parsed, nil
}
