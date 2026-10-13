// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASliceEndsBeforeItsJobTimesOut(t *testing.T) {
	t.Parallel()
	if importSliceBudget+importStopFlushTimeout >= importSliceTimeout {
		t.Fatalf("a slice of %v plus its stop flush of %v does not end inside its %v timeout",
			importSliceBudget, importStopFlushTimeout, importSliceTimeout)
	}
	// Only a claimed run can go stale, and a slice is claimed for at most its timeout.
	if importStaleAfter <= importSliceTimeout {
		t.Fatalf("a run is stale after %v, but one slice runs for %v: a live slice would read as dead",
			importStaleAfter, importSliceTimeout)
	}
}

func TestASliceBudgetEndsBySlicedRowsOrTime(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	got := importSliceBudgetOn(func() time.Time { return start })
	if got.Clock == nil || !got.Clock().Equal(start) {
		t.Fatal("the budget does not keep the slicer's clock")
	}
	got.Clock = nil
	want := migration.Budget{Rows: 500, Time: 12 * time.Second, FlushRows: 500, FlushEvery: 2 * time.Second}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("budget = %+v, want %+v", got, want)
	}
}

func TestEachImportAttemptIsItsOwnJob(t *testing.T) {
	t.Parallel()
	ws, run := ids.NewV7(), ids.NewV7()
	for _, args := range []importJobArgs{
		ImportCommitArgs{Workspace: ws, RunID: run, Attempt: 3},
		ImportUndoArgs{Workspace: ws, RunID: run, Attempt: 3},
	} {
		encoded, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		var keys map[string]any
		if err := json.Unmarshal(encoded, &keys); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"workspace_id", "run_id", "attempt"} {
			if _, ok := keys[key]; !ok {
				t.Errorf("%s args carry no %q key: %s", args.Kind(), key, encoded)
			}
		}
		opts := args.insertOpts()
		if opts.Queue != importWriteQueue || opts.MaxAttempts != 1 || !opts.UniqueOpts.ByArgs {
			t.Errorf("%s queues on %q with %d attempts, unique by args %v; want %q, 1, true",
				args.Kind(), opts.Queue, opts.MaxAttempts, opts.UniqueOpts.ByArgs, importWriteQueue)
		}
		if !slices.Equal(opts.UniqueOpts.ByState, activeSweepStates) {
			t.Errorf("%s dedupes over %v, want the in-flight states %v", args.Kind(), opts.UniqueOpts.ByState, activeSweepStates)
		}
		if lease := args.lease(); lease.Run != run || lease.Attempt != 3 {
			t.Errorf("%s leases %+v, want run %s attempt 3", args.Kind(), lease, run)
		}
	}
	if got := (ImportCommitArgs{}).lease().Status; got != migration.StatusRunning {
		t.Errorf("a commit leases a %s run, want %s", got, migration.StatusRunning)
	}
	if got := (ImportUndoArgs{}).lease().Status; got != migration.StatusUndoing {
		t.Errorf("an undo leases a %s run, want %s", got, migration.StatusUndoing)
	}
}

// The branches of settle that write nothing. The ones that record a cause or
// hand the attempt back to its queue need a run row, so a database tests them.
func TestASliceThatWritesNothingSettlesWithoutAFailure(t *testing.T) {
	t.Parallel()
	slicer := importSlicer{}
	for name, err := range map[string]error{
		"done":            nil,
		"a newer attempt": fmt.Errorf("flush: %w", migration.ErrSuperseded),
	} {
		result, err := slicer.settle(context.Background(), context.Background(), nil, migration.Lease{}, err)
		if err != nil || result != (importSliceResult{}) {
			t.Errorf("%s: settle = %+v, %v; want settled with no stop, nil", name, result, err)
		}
	}
	recorded := fmt.Errorf("walk: %w", migration.ErrStopRecorded)
	if result, err := slicer.settle(context.Background(), context.Background(), nil, migration.Lease{}, recorded); err != nil ||
		result != (importSliceResult{stopped: recorded}) {
		t.Errorf("a stop the engine recorded: settle = %+v, %v; want it settled and logged with no second write", result, err)
	}
}

func TestAStoppedRunCarriesASentenceNeverItsError(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"a lost grant":      {fmt.Errorf("claim: %w", apperrors.ErrPermissionDenied), migration.CausePermission},
		"a named stop":      {migration.RunStop{Cause: importTagsFailedCause, Err: errors.New("tag store: duplicate key")}, importTagsFailedCause},
		"anything else":     {errors.New("pq: relation import_record_map does not exist"), migration.CauseStopped},
		"a wrapped refusal": {fmt.Errorf("slice: %w", migration.RunStop{Cause: migration.CauseStorageUnavailable, Err: errNoObjectStore}), migration.CauseStorageUnavailable},
	} {
		if got := migration.StopCause(migration.StatusRunning, tc.err); got != tc.want {
			t.Errorf("%s: cause = %q, want %q", name, got, tc.want)
		}
	}
}

func TestACommitOnAWorkerWithNoObjectStoreStopsWithThatCause(t *testing.T) {
	t.Parallel()
	_, err := (&importCommitWorker{}).commit(context.Background(), importSlice{})
	if got := migration.StopCause(migration.StatusRunning, err); got != migration.CauseStorageUnavailable {
		t.Fatalf("cause = %q (err %v), want %q", got, err, migration.CauseStorageUnavailable)
	}
}

func TestATestRunOnAWorkerWithNoObjectStoreStopsWithThatCause(t *testing.T) {
	t.Parallel()
	_, err := (&importValidateWorker{}).validate(context.Background(), importSlice{})
	if got := migration.StopCause(migration.StatusValidating, err); got != migration.CauseTestStorageUnavailable {
		t.Fatalf("cause = %q (err %v), want %q", got, err, migration.CauseTestStorageUnavailable)
	}
}

func TestATestRunKeepsARefusalTheImporterCanActOn(t *testing.T) {
	t.Parallel()
	header := fmt.Errorf("reading leads.csv: %w", migration.ErrHeaderInvalid)
	if got := migration.StopCause(migration.StatusValidating, testRunRefusal(header)); got != header.Error() {
		t.Errorf("a broken header: cause = %q, want the refusal's own sentence %q", got, header.Error())
	}
	driver := errors.New("pq: relation import_record_map does not exist")
	if got := migration.StopCause(migration.StatusValidating, testRunRefusal(driver)); got != migration.CauseTestRunStopped {
		t.Errorf("a driver error: cause = %q, want %q", got, migration.CauseTestRunStopped)
	}
	for _, own := range []error{context.DeadlineExceeded, context.Canceled, migration.ErrSuperseded, apperrors.ErrPermissionDenied} {
		if wrapped := fmt.Errorf("reading: %w", own); !errors.Is(testRunRefusal(wrapped), own) || errors.As(testRunRefusal(wrapped), new(migration.RunStop)) {
			t.Errorf("%v: the refusal must stay the error the job settles by, not a sentence", own)
		}
	}
}

func TestARunPastItsTagPhaseCreatesNoTag(t *testing.T) {
	t.Parallel()
	slice := importSlice{run: migration.Run{Phase: migration.PhaseImporting}}
	// A nil resolver and a nil store: reaching either would panic.
	if err := createImportTags(context.Background(), slice, nil); err != nil {
		t.Fatalf("a run already importing ran its tag phase again: %v", err)
	}
}
