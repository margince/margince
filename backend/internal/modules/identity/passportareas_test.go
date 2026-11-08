// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAPassportsAreasRefuseWhatNoScreenOffers(t *testing.T) {
	for name, tc := range map[string]struct {
		areas principal.WorkAreas
		code  string
	}{
		"no area chosen":        {principal.WorkAreas{Limited: true}, "no_area_chosen"},
		"an area out of choice": {principal.WorkAreas{Limited: true, Chosen: []string{"operator"}}, "unknown_area"},
		"an unknown area":       {principal.WorkAreas{Limited: true, Chosen: []string{"deals"}}, "unknown_area"},
		"one area twice":        {principal.WorkAreas{Limited: true, Chosen: []string{"business", "business"}}, "area_repeated"},
	} {
		var bad *InvalidPassportFieldError
		if err := admitAreas(tc.areas); !errors.As(err, &bad) || bad.Field != "areas" || bad.Code != tc.code {
			t.Errorf("%s: %v, want a 422 on areas with code %s", name, err, tc.code)
		}
	}
	for name, areas := range map[string]principal.WorkAreas{
		"all areas":  {},
		"two chosen": {Limited: true, Chosen: []string{"business", "admin_settings"}},
	} {
		if err := admitAreas(areas); err != nil {
			t.Errorf("%s: %v, want admitted", name, err)
		}
	}
}

// NULL is all areas, so a passport minted before the column keeps its reach.
func TestAStoredNullIsAllAreasAndAListIsOnlyItsOwn(t *testing.T) {
	if got := areasFromColumn(nil); got.Limited {
		t.Fatalf("a NULL column reads as %+v, want all areas", got)
	}
	chosen := principal.WorkAreas{Limited: true, Chosen: []string{"compliance"}}
	if got := areasFromColumn(areasColumn(chosen)); !got.Limited || !slices.Equal(got.Chosen, chosen.Chosen) {
		t.Fatalf("a chosen list reads back as %+v", got)
	}
	if areasColumn(principal.WorkAreas{}) != nil {
		t.Fatal("all areas must be stored as NULL, so an area added later is reached")
	}
}

func TestTheWireCarriesTheModeAndTheChosenAreas(t *testing.T) {
	all := areasToWire(principal.WorkAreas{})
	if all.Mode != crmcontracts.PassportAreasModeAll || all.Areas == nil || len(all.Areas) != 0 {
		t.Fatalf("all areas goes out as %+v, want mode all and an empty list", all)
	}
	wire := crmcontracts.PassportAreas{Mode: crmcontracts.PassportAreasModeCustom, Areas: []crmcontracts.WorkArea{"business"}}
	read, err := areasFromWire(wire)
	if got := areasToWire(read); err != nil || got.Mode != wire.Mode || !slices.Equal(got.Areas, wire.Areas) {
		t.Fatalf("a custom choice round-trips as %+v, %v", got, err)
	}
	var bad *InvalidPassportFieldError
	allWithList := crmcontracts.PassportAreas{Mode: crmcontracts.PassportAreasModeAll, Areas: []crmcontracts.WorkArea{"business"}}
	if _, err := areasFromWire(allWithList); !errors.As(err, &bad) || bad.Code != "areas_with_all" {
		t.Errorf("all areas with a list → %v, want a 422 rather than the list dropped", err)
	}
}

func TestTheConsentFormPostsAllOrTheChosenAreas(t *testing.T) {
	for _, raw := range []string{"", "all", "  all "} {
		if got, err := parseConsentedAreas(raw); err != nil || got.Limited {
			t.Errorf("areas=%q reads as %+v, %v; want all areas", raw, got, err)
		}
	}
	got, err := parseConsentedAreas("business own_account")
	if err != nil || !got.Limited || !slices.Equal(got.Chosen, []string{"business", "own_account"}) {
		t.Fatalf("two areas read as %+v, %v", got, err)
	}
	for _, raw := range []string{"business agent_governance", "all business"} {
		if _, err := parseConsentedAreas(raw); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("areas=%q → %v, want the terminal refusal the consent screen shows", raw, err)
		}
	}
}
