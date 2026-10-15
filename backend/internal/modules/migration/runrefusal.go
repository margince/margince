// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Codes of the conflicts an import run gives a caller. The run id and the
// word "conflict" stay in the log line. The wire carries the code and a
// sentence a reader can act on.
const (
	// RunInFlightCode refuses a commit or undo while another attempt holds the run.
	RunInFlightCode = string(ReasonRunInFlight)
	// RunStateCode refuses a move the run's status does not allow.
	RunStateCode = "import_run_state"
)

// RunRefusal is a conflict raised for one import run. It answers as a message
// fault (409), so the caller reads no run id and no internal phrase.
type RunRefusal struct {
	run     RunID
	code    string
	message string
}

// Error is the log line, with the run it was about.
func (e *RunRefusal) Error() string {
	if e.run == (RunID{}) {
		return e.message
	}
	return fmt.Sprintf("import run %s: %s", e.run, e.message)
}

// Unwrap keeps the 409 the sentinel table gives.
func (e *RunRefusal) Unwrap() error { return apperrors.ErrConflict }

// MessageFault is the code and the sentence the caller reads.
func (e *RunRefusal) MessageFault() (string, string) { return e.code, e.message }

// RefuseRun is a RunStateCode conflict for the run. The sentence names no id.
func RefuseRun(id RunID, format string, args ...any) error {
	return &RunRefusal{run: id, code: RunStateCode, message: fmt.Sprintf(format, args...)}
}

// ErrRunWritesHeld refuses a commit or undo slice while another holds the run.
var ErrRunWritesHeld error = &RunRefusal{
	code:    RunInFlightCode,
	message: ReasonTemplate(ReasonRunInFlight, textlang.English),
}

// statusWords is a status as a reader says it.
func statusWords(status string) string { return strings.ReplaceAll(status, "_", " ") }
