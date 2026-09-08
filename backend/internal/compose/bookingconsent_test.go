// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The anonymous booking edge may assert consent for the booking-scoped
// `transactional` purpose only. A marketing (or any other) purpose, even
// though tracked, must be refused so a stranger who knows only a victim's
// email cannot plant an effective grant under it (F-008).
func TestAdmitBookingPurposeScopesToTransactional(t *testing.T) {
	transactional := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: bookingScopedPurposeKey}
	marketing := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: "marketing_email"}
	catalog := []consent.Purpose{transactional, marketing}

	if err := admitBookingPurpose(catalog, transactional.ID.UUID); err != nil {
		t.Fatalf("transactional purpose must be admitted, got %v", err)
	}

	if err := admitBookingPurpose(catalog, marketing.ID.UUID); err == nil {
		t.Fatal("a marketing purpose must be refused on the anonymous booking edge")
	} else if !isValidation(err, "consent.purpose_id") {
		t.Fatalf("out-of-scope purpose must be a consent.purpose_id validation fault, got %v", err)
	}

	unknown := ids.New[ids.PurposeKind]()
	if err := admitBookingPurpose(catalog, unknown.UUID); err == nil {
		t.Fatal("an untracked purpose id must be refused")
	} else if !isValidation(err, "consent.purpose_id") {
		t.Fatalf("unknown purpose must be a consent.purpose_id validation fault, got %v", err)
	}
}

// isValidation reports whether err is a 422 field-validation fault naming
// the given field — the booking edge's only client-fault shape here.
func isValidation(err error, field string) bool {
	var de *httperr.DetailedError
	if !errors.As(err, &de) || de.Code != "validation_error" {
		return false
	}
	for _, fieldErr := range de.Fields {
		if fieldErr.Field == field {
			return true
		}
	}
	return false
}

// The marketing tick admits a purpose by the PROPERTY that makes it safe —
// requires_double_opt_in — rather than by a named key, because a tick mails a
// question instead of writing a grant and an installation may run more than one
// newsletter. A purpose without the flag is grantable on assertion alone, which
// is what an anonymous door must not reach.
func TestAdmitBookingMarketingPurposeRequiresDoubleOptIn(t *testing.T) {
	doi := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: "marketing_email", RequiresDoubleOptIn: true}
	second := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: "product_news", RequiresDoubleOptIn: true}
	plain := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: bookingScopedPurposeKey}
	catalog := []consent.Purpose{doi, second, plain}

	// Two of them, so the test cannot pass by admitting one hard-coded key.
	for _, p := range []consent.Purpose{doi, second} {
		if err := admitBookingMarketingPurpose(catalog, p.ID.UUID); err != nil {
			t.Fatalf("a double-opt-in purpose must be admitted, got %v for %s", err, p.Key)
		}
	}

	if err := admitBookingMarketingPurpose(catalog, plain.ID.UUID); err == nil {
		t.Fatal("a purpose that does not require double opt-in must be refused")
	} else if !isValidation(err, "consent.marketing.purpose_id") {
		t.Fatalf("a non-DOI purpose must be a consent.marketing.purpose_id fault, got %v", err)
	}

	unknown := ids.New[ids.PurposeKind]()
	if err := admitBookingMarketingPurpose(catalog, unknown.UUID); err == nil {
		t.Fatal("an untracked purpose id must be refused")
	} else if !isValidation(err, "consent.marketing.purpose_id") {
		t.Fatalf("unknown purpose must be a consent.marketing.purpose_id fault, got %v", err)
	}
}

// The two admissions are not the same rule, and neither may stand in for the
// other. The operational lane takes exactly `transactional` and the marketing
// question refuses it; the marketing question takes a double-opt-in purpose and
// the operational lane refuses that. A future edit collapsing them into one
// check fails here.
func TestTheTwoBookingAdmissionsAdmitDisjointPurposes(t *testing.T) {
	transactional := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: bookingScopedPurposeKey}
	marketing := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: "marketing_email", RequiresDoubleOptIn: true}
	catalog := []consent.Purpose{transactional, marketing}

	if err := admitBookingMarketingPurpose(catalog, transactional.ID.UUID); err == nil {
		t.Fatal("the operational purpose must not be admissible as a marketing question")
	}
	if err := admitBookingPurpose(catalog, marketing.ID.UUID); err == nil {
		t.Fatal("the marketing purpose must not be admissible on the operational lane")
	}
}
