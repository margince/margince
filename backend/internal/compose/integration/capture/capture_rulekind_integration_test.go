// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
)

// A hold and an owner address take address or domain. A container, which only
// an exclusion takes, is refused on kind before the value is read. So the caller
// is never taught a value format the store then refuses.
func TestAHoldOrOwnerAddressRefusesAContainerNamingTheKindsItTakes(t *testing.T) {
	e := integration.SetupSearch(t)
	ctx := seatContext(e, e.Rep1)
	adds := map[string]func(kind, value string) error{
		"hold": func(kind, value string) error {
			_, err := capturemod.NewCounterpartyHoldStore(e.DB()).Add(ctx, kind, value)
			return err
		},
		"owner identity": func(kind, value string) error {
			_, err := capturemod.NewOwnerIdentityStore(e.DB()).Add(ctx, kind, value)
			return err
		},
	}
	for surface, add := range adds {
		for _, kind := range []string{"company", capturemod.ExclusionKindContainer} {
			err := add(kind, "gmail:Private")
			var invalid *capturemod.InvalidExclusionError
			if !errors.As(err, &invalid) || invalid.Field != "kind" || invalid.Reason != "kind is address or domain" {
				t.Errorf("%s with kind %q gave %v, want a refusal on kind reading %q",
					surface, kind, err, "kind is address or domain")
			}
		}
	}
}
