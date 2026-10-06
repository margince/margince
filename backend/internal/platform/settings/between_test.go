// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"strings"
	"testing"
)

func TestBetweenAdmitsItsEndsAndRefusesPastThem(t *testing.T) {
	valid := Between("seconds", 30, 600)
	for _, n := range []int{30, 600} {
		if err := valid(n); err != nil {
			t.Errorf("%d refused: %v", n, err)
		}
	}
	for _, n := range []int{29, 601} {
		err := valid(n)
		if err == nil {
			t.Errorf("%d admitted, want a refusal", n)
			continue
		}
		if want := "choose 30..600 seconds"; !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name the range and unit %q", err, want)
		}
	}
}
