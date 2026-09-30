// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// TestEachBoughtAttributeCarriesTheDateOfTheRunThatLastReportedIt: a newer run
// replaces the location and adds to the departments, so a value only the older
// run reported keeps the older date.
func TestEachBoughtAttributeCarriesTheDateOfTheRunThatLastReportedIt(t *testing.T) {
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	claim := func(key provider.ClaimKey, value string, at time.Time) storedClaim {
		return storedClaim{key: string(key), value: []byte(value), retrievedAt: at, provider: "surfe"}
	}
	profile := crmcontracts.ContactProviderProfile{Attributes: &[]crmcontracts.ContactProviderAttribute{}}
	if err := foldClaims([]storedClaim{
		claim(provider.ClaimLocation, `"Berlin, Germany"`, march),
		claim(provider.ClaimDepartments, `["Sales","Finance"]`, march),
		claim(provider.ClaimSeniorities, `["Director"]`, march),
		claim(provider.ClaimLocation, `"Munich, Germany"`, june),
		claim(provider.ClaimDepartments, `["Sales"]`, june),
	}, &profile); err != nil {
		t.Fatal(err)
	}

	type key struct {
		kind  crmcontracts.ContactProviderAttributeKind
		value string
	}
	got := map[key]time.Time{}
	for _, a := range *profile.Attributes {
		if _, twice := got[key{a.Kind, a.Value}]; twice {
			t.Errorf("%s %q is listed twice", a.Kind, a.Value)
		}
		got[key{a.Kind, a.Value}] = a.RetrievedAt
	}
	want := map[key]time.Time{
		{crmcontracts.ContactProviderAttributeKindLocation, "Munich, Germany"}: june,
		{crmcontracts.ContactProviderAttributeKindDepartment, "Sales"}:         june,
		{crmcontracts.ContactProviderAttributeKindDepartment, "Finance"}:       march,
		{crmcontracts.ContactProviderAttributeKindSeniority, "Director"}:       march,
	}
	if len(got) != len(want) {
		t.Errorf("got %d attributes, want %d (a replaced location must not linger): %v", len(got), len(want), got)
	}
	for k, at := range want {
		if !got[k].Equal(at) {
			t.Errorf("%s %q is dated %v, want %v", k.kind, k.value, got[k], at)
		}
	}
}
