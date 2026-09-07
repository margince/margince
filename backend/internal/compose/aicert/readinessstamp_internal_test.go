// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Internal because the subject is unexported: splitStampChange reads a stamp's
// three positions, and testing it through Readiness would need a whole census
// and records to reach one comparison.

import (
	"crypto/sha256"
	"strings"
	"testing"
)

// A stamp is three fixed-width digests, and which one moved decides what to do
// about it — so the split is held against each position and against a stamp it
// cannot read.
func TestASplitStampNamesTheHalfThatMoved(t *testing.T) {
	t.Parallel()

	seg := func(b byte) string { return strings.Repeat(string(b), sha256.Size*2) }
	base := seg('a') + seg('b') + seg('c')

	for name, tc := range map[string]struct {
		now  string
		want string
	}{
		"the case alone":    {seg('z') + seg('b') + seg('c'), "the case"},
		"the prompt alone":  {seg('a') + seg('z') + seg('c'), "the prompt this build sends"},
		"the grader alone":  {seg('a') + seg('b') + seg('z'), "the grader"},
		"a case and prompt": {seg('z') + seg('z') + seg('c'), "the case and the prompt this build sends"},
		"all three":         {seg('z') + seg('z') + seg('z'), "the case and the prompt this build sends and the grader"},
		"a stamp too short": {seg('a'), "something this build cannot attribute"},
		"nothing moved":     {base, "something this build cannot attribute"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := splitStampChange(base, tc.now).Describe(); got != tc.want {
				t.Errorf("Describe() = %q, want %q — a reason that named the wrong half would send "+
					"somebody to re-certify a test nobody touched", got, tc.want)
			}
		})
	}
}
