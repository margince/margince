// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A pinned write against a row that moved names the row's version before the
// pin, so the reader knows which number to retry with.
func TestAMovedRowNamesItsVersionBeforeThePin(t *testing.T) {
	current, pinned := int64(5), int64(3)
	err := refuseIfVersionMoved("lead", &current, writeOptions{atVersion: &pinned})
	want := "This lead is at version 5, not version 3, so nothing was changed. " +
		"Read it again, then make the change again if it still applies."
	if !errors.Is(err, apperrors.ErrVersionSkew) || err.Error() != want {
		t.Fatalf("a lead at 5 pinned to 3 answered %v, want version skew reading %q", err, want)
	}
}

// A row with no version can never satisfy a pin. The sentence points at the one
// call that still works: the change without a version condition.
func TestAVersionlessRowRefusesAPinAndSaysWhatStillWorks(t *testing.T) {
	pinned := int64(3)
	err := refuseIfVersionMoved("lead", nil, writeOptions{atVersion: &pinned})
	want := "This lead has no version, so a change conditioned on version 3 cannot be made to it. " +
		"Nothing was changed. Make the change without a version condition."
	if !errors.Is(err, apperrors.ErrVersionSkew) || err.Error() != want {
		t.Fatalf("a versionless lead pinned to 3 answered %v, want version skew reading %q", err, want)
	}
	if err := refuseIfVersionMoved("lead", nil, writeOptions{}); err != nil {
		t.Fatalf("the same versionless lead with no pin answered %v, want the write to proceed", err)
	}
}
