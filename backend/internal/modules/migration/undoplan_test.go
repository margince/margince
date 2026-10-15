// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestUndoPlanFollowsTheCheckpointAcrossObjects(t *testing.T) {
	plan := undoPlan{
		{Object: "activity", RowsTotal: 3},
		{Object: "contact", RowsTotal: 2},
		{Object: "company", RowsTotal: 4},
	}
	cases := []struct {
		processed int
		done      []int
		current   string
	}{
		{0, []int{0, 0, 0}, "activity"},
		{3, []int{3, 0, 0}, "contact"},
		{4, []int{3, 1, 0}, "contact"},
		{9, []int{3, 2, 4}, ""},
		{40, []int{3, 2, 4}, ""},
	}
	for _, c := range cases {
		progress, current := plan.at(c.processed)
		var got []int
		for _, one := range progress.Objects {
			got = append(got, one.RowsDone)
		}
		if fmt.Sprint(got) != fmt.Sprint(c.done) || current != c.current {
			t.Errorf("at(%d) = done %v on %q, want %v on %q", c.processed, got, current, c.done, c.current)
		}
	}
}

func TestRunInFlightIsCodedAndNamesNoRun(t *testing.T) {
	err := fmt.Errorf("import run %s: %w", ids.NewV7(), ErrRunWritesHeld)
	if !errors.Is(err, ErrRunWritesHeld) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("the refusal must stay matchable and a conflict: %v", err)
	}
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != 409 || fault.Code != RunInFlightCode {
		t.Fatalf("Classify = %+v, %v; want 409 %s", fault, ok, RunInFlightCode)
	}
	if strings.Contains(fault.Detail, "import run") || strings.Contains(fault.Detail, "conflict") {
		t.Errorf("detail leaks internals: %q", fault.Detail)
	}
	if want := ReasonTemplate(ReasonRunInFlight, textlang.English); fault.Detail != want {
		t.Errorf("detail = %q, want the catalog sentence %q", fault.Detail, want)
	}
}

func TestRunStateRefusalHidesTheRunID(t *testing.T) {
	id := RunID(ids.NewV7())
	fault, ok := httperr.Classify(refusedStart(id, StatusComplete, StatusRunning))
	if !ok || fault.Status != 409 || fault.Code != RunStateCode {
		t.Fatalf("Classify = %+v, %v", fault, ok)
	}
	if strings.Contains(fault.Detail, id.String()) {
		t.Errorf("detail names the run: %q", fault.Detail)
	}
}
