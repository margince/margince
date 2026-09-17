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

	if got, err := admitBookingPurpose(catalog, &transactional.ID.UUID); err != nil {
		t.Fatalf("transactional purpose must be admitted, got %v", err)
	} else if got != transactional.ID.UUID {
		t.Fatalf("admitted purpose = %v, want the one named %v", got, transactional.ID.UUID)
	}

	if _, err := admitBookingPurpose(catalog, &marketing.ID.UUID); err == nil {
		t.Fatal("a marketing purpose must be refused on the anonymous booking edge")
	} else if !isValidation(err, "consent.purpose_id") {
		t.Fatalf("out-of-scope purpose must be a consent.purpose_id validation fault, got %v", err)
	}

	unknown := ids.New[ids.PurposeKind]()
	if _, err := admitBookingPurpose(catalog, &unknown.UUID); err == nil {
		t.Fatal("an untracked purpose id must be refused")
	} else if !isValidation(err, "consent.purpose_id") {
		t.Fatalf("unknown purpose must be a consent.purpose_id validation fault, got %v", err)
	}
}

// A form that names NO purpose is the case a published page is actually in:
// purpose ids are per-installation uuids minted at seed time and the contract
// exposes no anonymous read of them, so the page has nothing true to send. The
// door is confined to one purpose anyway, so it answers that one.
//
// This is the defect the whole change is about. A page shipped with a stand-in
// id refused on every installation there is, because no installation could hold
// that id by chance.
func TestAnUnnamedBookingPurposeResolvesToTheBookingLane(t *testing.T) {
	transactional := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: bookingScopedPurposeKey}
	marketing := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: "marketing_email", RequiresDoubleOptIn: true}
	catalog := []consent.Purpose{marketing, transactional}

	got, err := admitBookingPurpose(catalog, nil)
	if err != nil {
		t.Fatalf("a booking naming no purpose must resolve the lane, got %v", err)
	}
	if got != transactional.ID.UUID {
		t.Fatalf("resolved purpose = %v, want the transactional lane %v", got, transactional.ID.UUID)
	}
}

// And it resolves by KEY rather than by position: a catalog whose only entries
// are other purposes must refuse rather than hand back whichever one came
// first, which is how a grant would land on a marketing purpose nobody asked
// about.
func TestAnUnnamedBookingPurposeRefusesACatalogWithoutTheLane(t *testing.T) {
	marketing := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: "marketing_email", RequiresDoubleOptIn: true}
	correspondence := consent.Purpose{ID: ids.New[ids.PurposeKind](), Key: "business_correspondence"}

	got, err := admitBookingPurpose([]consent.Purpose{marketing, correspondence}, nil)
	if err == nil {
		t.Fatalf("a catalog with no transactional lane must refuse, got purpose %v", got)
	}
	if !isValidation(err, "consent.purpose_id") {
		t.Fatalf("a missing lane must be a consent.purpose_id validation fault, got %v", err)
	}
	if got != ids.Nil {
		t.Fatalf("a refusal answered purpose %v, want none", got)
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
	if _, err := admitBookingPurpose(catalog, &marketing.ID.UUID); err == nil {
		t.Fatal("the marketing purpose must not be admissible on the operational lane")
	}
}
