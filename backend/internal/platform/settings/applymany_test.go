// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

// ApplyManyTx's admission, which is the half a database cannot show.

import (
	"context"
	"strings"
	"testing"
)

// An entry not declared MachineryApplied refuses the WHOLE batch, before any
// read. Skipping it instead would answer a partial policy — the same
// half-applied state the batch exists to prevent, reached by a different route.
func TestApplyManyRefusesTheBatchForOneUndeclaredEntry(t *testing.T) {
	declared := Define[bool]("t.declared", "obj", "update", false, nil).MachineryApplied()
	ordinary := Define[int]("t.ordinary", "obj", "update", 0, nil)

	// A nil transaction is safe here and is the point: the admission is asked
	// before the read, so a refused batch must never reach one.
	_, err := ApplyManyTx(context.Background(), nil, declared, ordinary)
	if err == nil {
		t.Fatal("a batch naming an undeclared entry was accepted, so the read would have run")
	}
	if !strings.Contains(err.Error(), ordinary.Key()) {
		t.Errorf("the refusal does not name %s: %v", ordinary.Key(), err)
	}
	if !strings.Contains(err.Error(), "MachineryApplied") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
}

// The order of the entries does not decide which refusal a caller gets: an
// undeclared entry anywhere in the batch refuses it.
func TestApplyManyRefusesWhicheverPositionTheUndeclaredEntryHolds(t *testing.T) {
	declared := Define[bool]("t.declared", "obj", "update", false, nil).MachineryApplied()
	ordinary := Define[int]("t.ordinary", "obj", "update", 0, nil)

	for _, tc := range []struct {
		name string
		defs []Definition
	}{
		{"first", []Definition{ordinary, declared}},
		{"last", []Definition{declared, ordinary}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ApplyManyTx(context.Background(), nil, tc.defs...); err == nil {
				t.Fatal("the batch was accepted with an undeclared entry in it")
			} else if !strings.Contains(err.Error(), ordinary.Key()) {
				t.Errorf("the refusal does not name %s: %v", ordinary.Key(), err)
			}
		})
	}
}
