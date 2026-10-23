// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import "testing"

func TestABandThisContractDoesNotDeclareReachesNoReader(t *testing.T) {
	if got := relationshipBand("legendary"); got != nil {
		t.Fatalf("an undeclared band reached the wire as %q", *got)
	}
	for _, bucket := range []string{"none", "weak", "moderate", "strong"} {
		if got := relationshipBand(bucket); got == nil || string(*got) != bucket {
			t.Fatalf("the §4 bucket %q did not reach the wire", bucket)
		}
	}
}
