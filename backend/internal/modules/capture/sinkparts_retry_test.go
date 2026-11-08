// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"
)

type stagedRetryMarker struct{ call int }

func (*stagedRetryMarker) StagedFile() {}

type retryStageBoundary struct {
	FileKeeper
	calls int
	fail  error
}

func (k *retryStageBoundary) Stage(context.Context, []CapturedFile) ([]StagedFile, error) {
	k.calls++
	return []StagedFile{&stagedRetryMarker{call: k.calls}}, k.fail
}

func TestAFileStageRetryReusesOnlyTheSameBytesAndMetadata(t *testing.T) {
	keeper := &retryStageBoundary{}
	sink := (&Sink{files: keeper}).withRetryFileStage()
	files := []CapturedFile{{PartID: "one", Filename: "note.txt", Body: []byte("original")}}
	first, err := sink.files.Stage(t.Context(), files)
	if err != nil {
		t.Fatal(err)
	}
	again, err := sink.files.Stage(t.Context(), files)
	if err != nil || keeper.calls != 1 || first[0] != again[0] {
		t.Fatalf("retry: calls=%d error=%v", keeper.calls, err)
	}
	files[0].Body[0] = 'X'
	if _, err := sink.files.Stage(t.Context(), files); err != nil {
		t.Fatal(err)
	}
	if keeper.calls != 2 {
		t.Fatal("changed bytes reused the old object")
	}
	files[0].Filename = "renamed.txt"
	if _, err := sink.files.Stage(t.Context(), files); err != nil {
		t.Fatal(err)
	}
	if keeper.calls != 3 {
		t.Fatal("changed metadata reused the old object")
	}
	other := (&Sink{files: keeper}).withRetryFileStage()
	if _, err := other.files.Stage(t.Context(), files); err != nil {
		t.Fatal(err)
	}
	if keeper.calls != 4 {
		t.Fatal("file staging escaped its capture call")
	}
}

func TestAFailedFileStageIsAttemptedAgain(t *testing.T) {
	keeper := &retryStageBoundary{fail: errors.New("object store unavailable")}
	sink := (&Sink{files: keeper}).withRetryFileStage()
	files := []CapturedFile{{PartID: "one", Body: []byte("body")}}
	if _, err := sink.files.Stage(t.Context(), files); err == nil {
		t.Fatal("stage failure disappeared")
	}
	keeper.fail = nil
	if _, err := sink.files.Stage(t.Context(), files); err != nil {
		t.Fatal(err)
	}
	if keeper.calls != 2 {
		t.Fatalf("stage calls=%d, want two", keeper.calls)
	}
}
