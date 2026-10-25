// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

func TestSteeringTrimsBothFieldsAndTreatsEmptyAsAbsent(t *testing.T) {
	t.Parallel()
	padded, blank := "  shorter \n", " \t"
	got := draftfloor.NewSteering(&padded, &padded)
	if got.Intent != "shorter" || got.RewriteOf != "shorter" {
		t.Errorf("trimmed steering = %+v, want both fields %q", got, "shorter")
	}
	if got := draftfloor.NewSteering(&blank, &blank); got != (draftfloor.Steering{}) {
		t.Errorf("blank fields = %+v, want a first draft with no ask", got)
	}
	if got := draftfloor.NewSteering(nil, nil); got != (draftfloor.Steering{}) {
		t.Errorf("absent fields = %+v, want zero", got)
	}
}
