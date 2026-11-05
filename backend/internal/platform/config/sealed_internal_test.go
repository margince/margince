// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// retiredBy is a vault double that records which blobs a swap retires.
func retiredBy() (*Sealer, *[]string) {
	retired := &[]string{}
	return &Sealer{Retire: func(_ context.Context, ref string) { *retired = append(*retired, ref) }}, retired
}

func TestARolledBackSwapRetiresTheNewBlobAndKeepsTheOld(t *testing.T) {
	sealer, retired := retiredBy()
	refused := errors.New("the write was refused")
	err := settleSwap(context.Background(), sealer, "test.sealed", swapOutcome{
		previous: "ref-old", next: "ref-new", err: refused,
	})
	if !errors.Is(err, refused) {
		t.Fatalf("settle = %v, want the refusal", err)
	}
	if !slices.Equal(*retired, []string{"ref-new"}) {
		t.Errorf("retired %v, want only the new blob: the row still names the old one", *retired)
	}
}

func TestAnUnclearCommitKeepsTheNewBlob(t *testing.T) {
	sealer, retired := retiredBy()
	lost := errors.New("the connection dropped during commit")
	err := settleSwap(context.Background(), sealer, "test.sealed", swapOutcome{
		previous: "ref-old", next: "ref-new", bodyDone: true, err: lost,
	})
	if !errors.Is(err, lost) {
		t.Fatalf("settle = %v, want the commit failure", err)
	}
	if len(*retired) != 0 {
		t.Errorf("retired %v after a commit that may have landed, want nothing", *retired)
	}
}

func TestACommittedSwapRetiresTheReplacedBlob(t *testing.T) {
	sealer, retired := retiredBy()
	if err := settleSwap(context.Background(), sealer, "test.sealed", swapOutcome{
		previous: "ref-old", next: "ref-new", committed: true, bodyDone: true,
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if !slices.Equal(*retired, []string{"ref-old"}) {
		t.Errorf("retired %v, want the replaced blob", *retired)
	}
}
