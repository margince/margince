// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// previewWait is how long preview_import waits for its test run's report. Most
// pasted files are tested well inside it, so most answers carry the report.
const previewWait = 20 * time.Second

// previewPoll is how often the wait reads the run again.
const previewPoll = time.Second

// stillTesting is what a preview answers when its test outlasts the wait.
const stillTesting = "the test is still running: call read_import (view report) with this run_id " +
	"until state is no longer validating. awaiting_approval carries the report, pending carries " +
	"the error that stopped the test, cancelled means a newer preview replaced this run"

// testEnded is what a preview answers when its test ended with no report. A
// human stopped it, or a newer test of the same source_name replaced it.
const testEnded = "the test ended without a report (state %s): call preview_import again to test the file"

// WaitClock is what preview_import waits on between reads of its run.
type WaitClock interface {
	After(d time.Duration) <-chan time.Time
}

type realWaitClock struct{}

func (realWaitClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// importWait is how long, and on what clock, preview_import waits.
type importWait struct {
	clock  WaitClock
	within time.Duration
}

func defaultImportWait() importWait {
	return importWait{clock: realWaitClock{}, within: previewWait}
}

// WithImportWait sets how long preview_import waits for its report. A nil
// clock keeps the real one.
func WithImportWait(clock WaitClock, within time.Duration) RegistryOption {
	return func(r *Registry) {
		if clock == nil {
			clock = realWaitClock{}
		}
		r.importWait = importWait{clock: clock, within: within}
	}
}

// awaitReport reads run until its test ends, the wait runs out or the caller
// leaves. Each read is its own transaction, so a wait holds no connection.
func (w importWait) awaitReport(
	ctx context.Context, imports Imports, run crmcontracts.ImportRun,
) (crmcontracts.ImportRun, error) {
	for waited := time.Duration(0); run.Status == crmcontracts.ImportRunStatusValidating && waited < w.within; waited += previewPoll {
		select {
		case <-ctx.Done():
			return run, nil
		case <-w.clock.After(previewPoll):
		}
		read, err := imports.ReadRun(ctx, ids.UUID(run.Id))
		if err != nil {
			return crmcontracts.ImportRun{}, err
		}
		run = read
	}
	return run, nil
}

// answered waits for staged's test and answers preview with the run it reached
// and, at awaiting_approval, its report. A test that refused the file answers
// that refusal, as the call would have had the test run inside it.
func (t previewImport) answered(
	ctx context.Context, staged crmcontracts.ImportRun, preview ImportPreviewResult,
) (json.RawMessage, error) {
	run, err := t.wait.awaitReport(ctx, t.imports, staged)
	if err != nil {
		return nil, err
	}
	switch {
	case run.Status == crmcontracts.ImportRunStatusAwaitingApproval:
		run, preview.Report, err = t.imports.ReadReport(ctx, ids.UUID(run.Id))
		if err != nil {
			return nil, err
		}
	case run.Status == crmcontracts.ImportRunStatusPending && run.Error != nil:
		return nil, fmt.Errorf("the test of import run %s refused it: %s: %w",
			run.Id, *run.Error, apperrors.ErrInvalidArgument)
	case run.Status == crmcontracts.ImportRunStatusValidating:
		preview.Next = stillTesting
	default:
		preview.Next = fmt.Sprintf(testEnded, run.Status)
	}
	preview.Run = importRunResult(run)
	return json.Marshal(preview)
}
