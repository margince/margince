// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability_test

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/pkg/extension"
)

// ledger is the database boundary of an object call. It holds one unit's
// usage and the changes logged, and refuses to record when down is set.
type ledger struct {
	held   extcapability.Usage
	logged []extcapability.ObjectChange
	down   bool
}

func (l *ledger) run(_ context.Context, _ string, plan func(extcapability.Usage) (extcapability.ObjectChange, error), write func() error) error {
	change, err := plan(l.held)
	if err != nil {
		return err
	}
	if l.down {
		return extcapability.ErrObjectsFailed
	}
	if err := write(); err != nil {
		return err
	}
	l.logged = append(l.logged, change)
	l.held.Objects += change.Usage.Objects
	l.held.Bytes += change.Usage.Bytes
	return nil
}

func objectsOver(store blobstore.Store, l *ledger) extcapability.Objects {
	return extcapability.Objects{Store: store, Ledger: l.run, Unit: "u", Gate: allow}
}

func TestEachPutAndDeleteIsLoggedAndCounted(t *testing.T) {
	t.Parallel()
	ctx, _ := inWorkspace()
	l := &ledger{}
	objects := objectsOver(blobstore.NewMemory(), l)
	for _, step := range []func() error{
		func() error { return objects.Put(ctx, "report", []byte("hello")) },
		func() error { return objects.Put(ctx, "report", []byte("hi")) },
		func() error { return objects.Delete(ctx, "report") },
		func() error { return objects.Delete(ctx, "report") },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	want := []extcapability.ObjectChange{
		{Action: extcapability.ObjectPut, Name: "report", Size: 5, Usage: extcapability.Usage{Objects: 1, Bytes: 5}},
		{Action: extcapability.ObjectPut, Name: "report", Size: 2, Usage: extcapability.Usage{Bytes: -3}},
		{Action: extcapability.ObjectDelete, Name: "report", Size: 2, Usage: extcapability.Usage{Objects: -1, Bytes: -2}},
		{Action: extcapability.ObjectDelete, Name: "report"},
	}
	if len(l.logged) != len(want) {
		t.Fatalf("logged %+v, want %+v", l.logged, want)
	}
	for i := range want {
		if l.logged[i] != want[i] {
			t.Errorf("change %d = %+v, want %+v", i, l.logged[i], want[i])
		}
	}
	if l.held != (extcapability.Usage{}) {
		t.Errorf("usage after put, replace and delete = %+v, want none", l.held)
	}
}

func TestAPutThatCannotBeLoggedStoresNothing(t *testing.T) {
	t.Parallel()
	ctx, ws := inWorkspace()
	store := blobstore.NewMemory()
	if err := objectsOver(store, &ledger{down: true}).Put(ctx, "report", []byte("x")); !errors.Is(err, extcapability.ErrObjectsFailed) {
		t.Errorf("Put with the log down: err = %v, want ErrObjectsFailed", err)
	}
	if _, _, err := store.Get(ctx, extcapability.Prefix(ws, "u")+"report"); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the object was stored though its log row was not: err = %v", err)
	}
}

func TestAUnitAtItsQuotaMayReplaceButNotAdd(t *testing.T) {
	t.Parallel()
	ctx, _ := inWorkspace()
	store := blobstore.NewMemory()
	l := &ledger{}
	objects := objectsOver(store, l)
	if err := objects.Put(ctx, "kept", []byte("abc")); err != nil {
		t.Fatal(err)
	}
	l.held.Objects = extension.MaxObjectsPerUnit
	if err := objects.Put(ctx, "new", []byte("x")); !errors.Is(err, extension.ErrObjectQuotaReached) {
		t.Errorf("a new object at the count cap: err = %v, want ErrObjectQuotaReached", err)
	}
	if err := objects.Put(ctx, "kept", []byte("abcd")); err != nil {
		t.Errorf("replacing an object at the count cap: %v", err)
	}
	l.held.Bytes = extension.MaxObjectBytesPerUnit
	if err := objects.Put(ctx, "kept", []byte("abcde")); !errors.Is(err, extension.ErrObjectQuotaReached) {
		t.Errorf("growing an object past the byte cap: err = %v, want ErrObjectQuotaReached", err)
	}
	if err := objects.Put(ctx, "kept", []byte("a")); err != nil {
		t.Errorf("shrinking an object at the byte cap: %v", err)
	}
}
