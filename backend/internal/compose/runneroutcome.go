// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
)

// outcomeCheckFor is the check a scheduled agent's final answer must pass, nil
// for an agent whose work does not log once per listed subject. It reads the
// same work the certification case grades (agentLoopWorks), so the two agree.
func outcomeCheckFor(spec runner.AgentSpec) runner.OutcomeCheck {
	work, known := agentLoopWorks[spec.Name]
	if !known || !work.onceEach {
		return nil
	}
	tools := spec.Tools
	return func(calls []runner.Call) (string, error) {
		write, err := writeTool(tools, agentLoopRegistry())
		if err != nil {
			return "", err
		}
		work.write = write
		return work.unwritten(calls)
	}
}

// unwritten names each subject the run's own list reads returned that no
// executed write names. The list is the tool's whole answer, not the trace's
// cut-down copy, and the model's own arguments chose it.
//
// A subject written twice is not refused. The store keeps one run note per
// deal a day and answers a repeat with the earlier note. No step could undo the
// repeat, so refusing it would only spend the budget.
func (w agentLoopWork) unwritten(calls []runner.Call) (string, error) {
	var listed, written []string
	for _, call := range calls {
		switch call.Tool {
		case w.list:
			if w.served == nil {
				return "", fmt.Errorf("outcome check: nothing reads a live %s answer", w.list)
			}
			subjects, err := w.served(call.Answer)
			if err != nil {
				return "", fmt.Errorf("outcome check: %w", err)
			}
			listed = append(listed, subjects...)
		case w.write:
			subjects, err := w.wrote(call.Args, agentLoopWorklist{})
			if err != nil {
				return "", fmt.Errorf("outcome check: an executed %s call does not read back: %w", w.write, err)
			}
			written = append(written, subjects...)
		}
	}
	var missing []string
	for _, tally := range runner.NotWrittenOnce(listed, written) {
		if tally.Writes == 0 {
			missing = append(missing, tally.Subject)
		}
	}
	if len(missing) == 0 {
		return "", nil
	}
	return fmt.Sprintf("%s listed these, and no %s call names them: %s",
		w.list, w.write, strings.Join(missing, ", ")), nil
}
