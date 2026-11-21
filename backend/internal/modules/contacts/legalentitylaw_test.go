// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// entityLawPack stands in for a compiled-in pack. A further entity may only
// name a pack the binary has, and this test binary composes no extension.
type entityLawPack struct{ code jurisdiction.Code }

func (p entityLawPack) Code() jurisdiction.Code { return p.code }

func (entityLawPack) Retention() jurisdiction.Retention { return nil }

func init() {
	jurisdiction.Register(entityLawPack{code: "at"})
	jurisdiction.Register(entityLawPack{code: "de"})
}

func TestTheDefaultEntityTakesNoJurisdictionOfItsOwn(t *testing.T) {
	t.Parallel()
	law := "de"
	_, _, err := legalEntityPatch(LegalEntity{IsDefault: true}, LegalEntityInput{Jurisdiction: &law})
	if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Code != codeFieldOnInstallation || fault.Field != wireJurisdiction {
		t.Errorf("a jurisdiction on the default entity answered %v, want %s on %s", err, codeFieldOnInstallation, wireJurisdiction)
	}
}

func TestAFurtherEntityNamesOnlyAJurisdictionTheBuildOffers(t *testing.T) {
	t.Parallel()
	for law, wantErr := range map[string]bool{"DE": false, "at": false, "zz": true, "fr": true} {
		_, changed, err := legalEntityPatch(LegalEntity{}, LegalEntityInput{Jurisdiction: &law})
		fault, refused := errors.AsType[*values.ParseError](err)
		if wantErr && (!refused || fault.Code != "invalid_jurisdiction") {
			t.Errorf("jurisdiction %q answered %v, want invalid_jurisdiction", law, err)
		}
		if !wantErr && (err != nil || len(changed) != 1) {
			t.Errorf("jurisdiction %q answered %v, changed %v; want it stored", law, err, changed)
		}
	}
	cleared := ""
	if _, _, err := legalEntityPatch(LegalEntity{}, LegalEntityInput{Jurisdiction: &cleared}); err == nil {
		t.Error("clearing a further entity's jurisdiction was accepted")
	}
}

func TestACreateWithoutAJurisdictionTakesTheCountry(t *testing.T) {
	t.Parallel()
	country, law := " AT ", "de"
	if got := (LegalEntityInput{Country: &country}).withDefaultJurisdiction(); got.Jurisdiction == nil || *got.Jurisdiction != "at" {
		t.Errorf("a create naming only the country got jurisdiction %v, want at", got.Jurisdiction)
	}
	if got := (LegalEntityInput{Country: &country, Jurisdiction: &law}).withDefaultJurisdiction(); *got.Jurisdiction != "de" {
		t.Errorf("a stated jurisdiction became %q, want de", *got.Jurisdiction)
	}
}

// The floor follows the law: a new address country alone carries nothing, and
// a new jurisdiction carries the old floor.
func TestTheFloorCarryFollowsTheJurisdictionNotTheAddress(t *testing.T) {
	t.Parallel()
	entity := func(country, law string) LegalEntity {
		return LegalEntity{Country: &country, Jurisdiction: &law}
	}
	var carried []EntityJurisdiction
	s := (&Store{}).WithFloorCarry(func(_ context.Context, _ pgx.Tx, from, to EntityJurisdiction) error {
		carried = append(carried, from, to)
		return nil
	})
	if err := s.carryFloor(context.Background(), nil, jurisdictionOf(entity("at", "at")), jurisdictionOf(entity("fr", "at"))); err != nil || len(carried) != 0 {
		t.Errorf("an address move carried %+v (%v), want nothing", carried, err)
	}
	if err := s.carryFloor(context.Background(), nil, jurisdictionOf(entity("at", "at")), jurisdictionOf(entity("at", "de"))); err != nil {
		t.Fatal(err)
	}
	if len(carried) != 2 || carried[0].Jurisdiction != "at" || carried[1].Jurisdiction != "de" {
		t.Errorf("a jurisdiction change carried %+v, want the floor carried from at to de", carried)
	}
}
