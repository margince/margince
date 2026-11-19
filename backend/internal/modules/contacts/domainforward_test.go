// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDomainIdentityForwardedUsesRegistrableDomains(t *testing.T) {
	for _, tc := range []struct {
		requested, effective string
		forwarded            bool
	}{
		{"acme.test", "https://www.acme.test/about", false},
		{"mail.acme.test", "https://acme.test/", false},
		{"alias.test", "https://other.test/", true},
		{"second.test", "https://other.test/", true},
		{"acme.test", "", false},
		{"acme.test", "https://192.0.2.1/", true},
		{"acme.test", "https:///missing-host", true},
		{"acme.test", "://malformed", true},
	} {
		t.Run(tc.requested+tc.effective, func(t *testing.T) {
			if got := DomainIdentityForwarded(tc.requested, tc.effective); got != tc.forwarded {
				t.Fatalf("forwarded = %v, want %v", got, tc.forwarded)
			}
		})
	}
}

// A redirect inside one registrable domain is no forwarded identity, so it is
// not withheld as one.
func TestARedirectWithinOneDomainIsNotWithheldAsForwarded(t *testing.T) {
	ctx := edgeGrantCtx(map[string]principal.ObjectGrant{"company": {Read: true, Update: true}})
	err := (&Store{}).WithholdForwardedDomain(ctx, DomainForwardEvidence{
		ReadID: ids.NewV7(), RequestedDomain: "acme.test", EffectiveURL: "https://www.acme.test/",
	})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("withholding a same-domain redirect = %v, want ErrInvalidArgument", err)
	}
}

// A vendor answer names a domain; an address or a URL is refused before any
// question is read.
func TestAVendorAnswerRefusesWhatIsNotADomain(t *testing.T) {
	ctx := edgeGrantCtx(map[string]principal.ObjectGrant{"company": {Read: true, Update: true}})
	if err := (&Store{}).KeepDomainAsVendor(ctx, "not a domain"); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("keeping %q as a vendor = %v, want ErrInvalidArgument", "not a domain", err)
	}
}
