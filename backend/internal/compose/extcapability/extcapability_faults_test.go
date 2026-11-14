// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

var errBoundary = errors.New("boundary down")

// failingLane answers every call with err.
type failingLane struct{ err error }

func (l failingLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, l.err
}

// failingStore refuses every write and delete; reads fall through to the memory store.
type failingStore struct{ blobstore.Store }

func (failingStore) Put(context.Context, string, io.Reader, int64, string) error {
	return errBoundary
}

func (failingStore) Delete(context.Context, string) error { return errBoundary }

func TestAModelCallReportsEachWayTheLaneOrTheTraceCanFail(t *testing.T) {
	t.Parallel()
	traceDown := func(context.Context) error { return errBoundary }
	traceOK := func(context.Context) error { return nil }
	for name, tc := range map[string]struct {
		models extcapability.Models
		req    extension.CompleteRequest
		want   error
	}{
		"restricted contact": {extcapability.Models{Lane: failingLane{model.ErrSubjectRestricted}, Gate: allow, Trace: traceOK}, oneMessage, extension.ErrForbidden},
		"lane fault":         {extcapability.Models{Lane: failingLane{errBoundary}, Gate: allow, Trace: traceOK}, oneMessage, extcapability.ErrModelFailed},
		"trace fault":        {extcapability.Models{Lane: &lane{}, Gate: allow, Trace: traceDown}, oneMessage, errBoundary},
		"invalid request":    {extcapability.Models{Lane: &lane{}, Gate: allow, Trace: traceOK}, extension.CompleteRequest{}, extension.ErrInvalid},
	} {
		if _, err := tc.models.Complete(context.Background(), tc.req); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestAnObjectCallReportsEachWayTheStoreOrTheNameCanFail(t *testing.T) {
	t.Parallel()
	ctx, _ := inWorkspace()
	memory := blobstore.NewMemory()
	broken := objectsFor("u", failingStore{memory})
	if err := broken.Put(ctx, "report", []byte("x")); !errors.Is(err, extcapability.ErrObjectsFailed) {
		t.Errorf("Put on a failing store: err = %v, want ErrObjectsFailed", err)
	}
	if err := broken.Delete(ctx, "report"); !errors.Is(err, extcapability.ErrObjectsFailed) {
		t.Errorf("Delete on a failing store: err = %v, want ErrObjectsFailed", err)
	}
	healthy := objectsFor("u", memory)
	if _, err := healthy.Get(ctx, "missing"); !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("Get of a missing object: err = %v, want ErrNotFound", err)
	}
	if err := healthy.Put(ctx, "bad name!", []byte("x")); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Put under an invalid name: err = %v, want ErrInvalid", err)
	}
	if err := healthy.Put(ctx, "big", make([]byte, extension.MaxObjectBytes+1)); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Put over the bound: err = %v, want ErrInvalid", err)
	}
	if err := objectsFor("u", nil).Put(ctx, "report", []byte("x")); !errors.Is(err, extcapability.ErrObjectsUnwired) {
		t.Errorf("Put with no store bound: err = %v, want ErrObjectsUnwired", err)
	}
	if err := healthy.Put(context.Background(), "report", []byte("x")); !errors.Is(err, extcapability.ErrObjectsFailed) {
		t.Errorf("Put with no workspace on the context: err = %v, want ErrObjectsFailed", err)
	}
}

func TestAnObjectTheStoreHoldsOverTheBoundIsNotHandedToAUnit(t *testing.T) {
	t.Parallel()
	ctx, ws := inWorkspace()
	memory := blobstore.NewMemory()
	big := make([]byte, extension.MaxObjectBytes+1)
	if err := memory.Put(ctx, extcapability.Prefix(ws, "u")+"big", bytes.NewReader(big), int64(len(big)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := objectsFor("u", memory).Get(ctx, "big"); !errors.Is(err, extcapability.ErrObjectsFailed) {
		t.Errorf("Get of an oversized stored object: err = %v, want ErrObjectsFailed", err)
	}
}
