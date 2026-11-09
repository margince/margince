// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

func TestSteeringTrimsFieldsAndTreatsEmptyAsAbsent(t *testing.T) {
	t.Parallel()
	padded, blank := "  shorter \n", " \t"
	got := draftfloor.NewSteering(&padded, &padded, &padded)
	if got.Intent != "shorter" || got.RewriteOf != "shorter" || got.Subject != "shorter" {
		t.Errorf("trimmed steering = %+v, want each field %q", got, "shorter")
	}
	if got := draftfloor.NewSteering(&blank, &blank, nil); got != (draftfloor.Steering{}) {
		t.Errorf("blank fields = %+v, want a first draft with no ask", got)
	}
	if got := draftfloor.NewSteering(nil, nil, nil); got != (draftfloor.Steering{}) {
		t.Errorf("absent fields = %+v, want zero", got)
	}
}
