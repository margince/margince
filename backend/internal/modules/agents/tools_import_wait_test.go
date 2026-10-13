// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// firingClock answers every wait at once and counts how often it was asked.
type firingClock struct{ asked *int }

func (c firingClock) After(time.Duration) <-chan time.Time {
	*c.asked++
	fired := make(chan time.Time, 1)
	fired <- time.Time{}
	return fired
}

// testedImports stages a run at validating, then answers one read per state in
// reads, holding the last.
type testedImports struct {
	stubImports
	run   ids.UUID
	reads []crmcontracts.ImportRun
	read  *int
}

func (s testedImports) StageRun(context.Context, crmcontracts.CreateImportRunRequest) (crmcontracts.ImportRun, error) {
	return crmcontracts.ImportRun{Id: openapi_types.UUID(s.run), Status: crmcontracts.ImportRunStatusValidating}, nil
}

func (s testedImports) ReadRun(context.Context, ids.UUID) (crmcontracts.ImportRun, error) {
	at := min(*s.read, len(s.reads)-1)
	*s.read++
	return s.reads[at], nil
}

func newTestedImports(states ...crmcontracts.ImportRun) testedImports {
	run := ids.NewV7()
	for i := range states {
		states[i].Id = openapi_types.UUID(run)
	}
	return testedImports{run: run, reads: states, read: new(int)}
}

func previewOnce(ctx context.Context, imports Imports, wait importWait) (ImportPreviewResult, error) {
	tool := previewImport{imports: imports, wait: wait}
	raw, err := tool.Handle(ctx, json.RawMessage(
		`{"object":"company","csv":"name\nAcme\n","mapping":{"name":"display_name"}}`))
	if err != nil {
		return ImportPreviewResult{}, err
	}
	var out ImportPreviewResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return ImportPreviewResult{}, err
	}
	return out, nil
}

func TestPreviewAnswersValidatingPastTheWait(t *testing.T) {
	asked := 0
	imports := newTestedImports(crmcontracts.ImportRun{Status: crmcontracts.ImportRunStatusAwaitingApproval})
	got, err := previewOnce(t.Context(), imports, importWait{clock: firingClock{asked: &asked}})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if got.Run.State != "validating" || !strings.Contains(got.Next, "read_import") ||
		!strings.Contains(got.Next, "awaiting_approval") {
		t.Fatalf("answer = %+v, want validating and the copy sending the caller to read_import", got)
	}
	if asked != 0 || *imports.read != 0 {
		t.Fatalf("a wait of 0 waited %d times and read %d times, want neither", asked, *imports.read)
	}
}

func TestPreviewWaitsUntilTheReportIsReady(t *testing.T) {
	asked := 0
	imports := newTestedImports(
		crmcontracts.ImportRun{Status: crmcontracts.ImportRunStatusValidating},
		crmcontracts.ImportRun{Status: crmcontracts.ImportRunStatusAwaitingApproval},
	)
	got, err := previewOnce(t.Context(), imports, importWait{clock: firingClock{asked: &asked}, within: previewWait})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if got.Run.State != "awaiting_approval" || got.Next != "" {
		t.Fatalf("answer = %+v, want the report ready and no copy to poll", got)
	}
	if asked != 2 || *imports.read != 2 {
		t.Fatalf("waited %d and read %d times, want 2 each: it stops at the report", asked, *imports.read)
	}
}

func TestPreviewStopsWaitingAtItsBound(t *testing.T) {
	asked := 0
	imports := newTestedImports(crmcontracts.ImportRun{Status: crmcontracts.ImportRunStatusValidating})
	got, err := previewOnce(t.Context(), imports, importWait{clock: firingClock{asked: &asked}, within: 3 * previewPoll})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if got.Run.State != "validating" || got.Next == "" || *imports.read != 3 {
		t.Fatalf("answer = %+v after %d reads, want validating with the copy after 3", got, *imports.read)
	}
}

func TestPreviewStopsWaitingWhenTheCallerLeaves(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	imports := newTestedImports(crmcontracts.ImportRun{Status: crmcontracts.ImportRunStatusAwaitingApproval})
	blocked := importWait{clock: neverClock{}, within: previewWait}
	got, err := previewOnce(ctx, imports, blocked)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if got.Run.State != "validating" || *imports.read != 0 {
		t.Fatalf("answer = %+v after %d reads, want the run as staged", got, *imports.read)
	}
}

// neverClock never fires, so only the caller's context ends a wait.
type neverClock struct{}

func (neverClock) After(time.Duration) <-chan time.Time { return nil }

func TestARefusedTestAnswersItsReason(t *testing.T) {
	asked := 0
	reason := "the file has no header row"
	imports := newTestedImports(crmcontracts.ImportRun{Status: crmcontracts.ImportRunStatusPending, Error: &reason})
	_, err := previewOnce(t.Context(), imports, importWait{clock: firingClock{asked: &asked}, within: previewWait})
	if err == nil || !strings.Contains(err.Error(), reason) || !strings.Contains(err.Error(), imports.run.String()) {
		t.Fatalf("err = %v, want the refusal naming run %s and its reason", err, imports.run)
	}
}

func TestPreviewWaitEndsBeforeTheCallsDeadline(t *testing.T) {
	if previewWait+previewPoll >= mcpCallDeadline {
		t.Fatalf("previewWait %s plus one read %s reaches /mcp's deadline %s", previewWait, previewPoll, mcpCallDeadline)
	}
}
