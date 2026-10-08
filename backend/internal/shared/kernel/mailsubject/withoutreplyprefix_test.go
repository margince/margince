// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailsubject_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

func TestWithoutReplyPrefixKeepsTheCaseOfWhatRemains(t *testing.T) {
	for in, want := range map[string]string{
		"Re: Renewal terms":         "Renewal terms",
		"AW: RE: Renewal terms":     "Renewal terms",
		"  antw:Renewal terms ":     "Renewal terms",
		"Fwd: Renewal terms":        "Fwd: Renewal terms",
		"Renewal terms: Re: update": "Renewal terms: Re: update",
	} {
		if got := mailsubject.WithoutReplyPrefix(in); got != want {
			t.Errorf("WithoutReplyPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
