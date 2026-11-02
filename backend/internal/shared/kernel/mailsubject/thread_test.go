// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailsubject

import "testing"

func TestThreadSubjectsStripTheSharedReplyAndForwardPrefixes(t *testing.T) {
	for _, subject := range []string{"Offer", "Re: Re: AW: Offer", "WG: Re: Offer", "Fwd:Offer", " Antw : WG: AW : Offer ", "Re:\tFwd: Offer"} {
		if got := ThreadNormalized(subject); got != "offer" {
			t.Errorf("%q normalized to %q", subject, got)
		}
	}
	for _, subject := range []string{"", "Re:", "Fwd: AW: WG:"} {
		if got := ThreadNormalized(subject); got != "" {
			t.Errorf("prefix-only subject %q normalized to %q", subject, got)
		}
	}
	if got := Normalized("Fwd: Offer"); got != "fwd: offer" {
		t.Fatalf("answer comparison stopped distinguishing forwards: %q", got)
	}
}
