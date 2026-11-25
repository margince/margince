// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import "encoding/json"

// Call is one call the tool surface executed, with its whole answer. A check
// reads it here because a trace Step keeps only the first bytes of an answer.
type Call struct {
	Tool   string
	Args   json.RawMessage
	Answer json.RawMessage
}

// OutcomeCheck judges a final answer against the calls the run executed. It
// answers what the run left undone, or "" to accept the answer. An error is a
// call it could not read, and faults the run.
type OutcomeCheck func(calls []Call) (undone string, err error)

// Tally is one listed subject and how many writes named it.
type Tally struct {
	Subject string
	Writes  int
}

// NotWrittenOnce is every listed subject that one write does not name, in
// list order: unwritten, or written more than once. It is the one spelling of
// "one write per listed subject": the runner's outcome check, the certification
// grade and the live lane all read it.
func NotWrittenOnce(listed, written []string) []Tally {
	counts := make(map[string]int, len(written))
	for _, subject := range written {
		counts[subject]++
	}
	seen := make(map[string]bool, len(listed))
	var off []Tally
	for _, subject := range listed {
		if seen[subject] {
			continue
		}
		seen[subject] = true
		if n := counts[subject]; n != 1 {
			off = append(off, Tally{Subject: subject, Writes: n})
		}
	}
	return off
}

// outcomeCheckSource attributes the refusal of a final answer to the runner.
const outcomeCheckSource = "outcome_check"

// outcomeRefusedDirective follows the check's finding, outside its span.
const outcomeRefusedDirective = "Your final answer was not accepted. Do the work named above, then return your final answer again."

// recordCall keeps an executed call for the job's outcome check. A job with no
// check keeps nothing, so a run holds no second copy of answers nobody reads.
func (acc *Result) recordCall(job Job, tool string, args, answer json.RawMessage) {
	if job.Outcome != nil {
		acc.calls = append(acc.calls, Call{Tool: tool, Args: args, Answer: answer})
	}
}

// refuseFinal reports whether the job's outcome check sends the final answer
// back. A refusal goes into the window, and the run continues on its budget.
func refuseFinal(job Job, win *window, calls []Call, meta Meta) (bool, error) {
	if job.Outcome == nil {
		return false, nil
	}
	undone, err := job.Outcome(calls)
	if err != nil || undone == "" {
		return false, err
	}
	if meta.Reject != nil {
		meta.Reject()
	}
	win.observeThen(outcomeCheckSource, undone, outcomeRefusedDirective)
	return true, nil
}
