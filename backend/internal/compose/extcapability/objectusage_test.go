// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/pkg/extension"
)

// ledger is the database boundary of an object call. It holds one unit's
// usage and the changes logged. When down is set its commit fails after the
// store call, as PoolLedger's does.
type ledger struct {
	held   extcapability.Usage
	logged []extcapability.ObjectChange
	down   bool
}

func (l *ledger) run(_ context.Context, _ string, plan func(extcapability.Usage) (extcapability.ObjectChange, error), write func() error) error {
	change, err := plan(l.held)
	if err != nil || change == (extcapability.ObjectChange{}) {
		return err
	}
	if err := write(); err != nil {
		return err
	}
	if l.down {
		return extcapability.ErrWrittenNotRecorded
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

// A count that does not commit after the store call leaves the usage high,
// never low: the Put's bytes are taken back, and the deleted object stays
// counted.
func TestAnObjectCallWhoseCountDoesNotCommitNeverLeavesAnObjectUncounted(t *testing.T) {
	t.Parallel()
	ctx, ws := inWorkspace()
	store := blobstore.NewMemory()
	key := extcapability.Prefix(ws, "u") + "report"
	l := &ledger{down: true}
	if err := objectsOver(store, l).Put(ctx, "report", []byte("x")); !errors.Is(err, extcapability.ErrObjectsFailed) {
		t.Errorf("Put whose count did not commit: err = %v, want ErrObjectsFailed", err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the uncounted object stayed in the store: err = %v", err)
	}
	if err := store.Put(ctx, key, strings.NewReader("abc"), 3, "text/plain"); err != nil {
		t.Fatal(err)
	}
	l.held = extcapability.Usage{Objects: 1, Bytes: 3}
	if err := objectsOver(store, l).Delete(ctx, "report"); !errors.Is(err, extcapability.ErrObjectsFailed) {
		t.Errorf("Delete whose count did not commit: err = %v, want ErrObjectsFailed", err)
	}
	if l.held != (extcapability.Usage{Objects: 1, Bytes: 3}) {
		t.Errorf("usage after an uncounted delete = %+v, want it still counted", l.held)
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
