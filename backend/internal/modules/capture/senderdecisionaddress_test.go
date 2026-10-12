// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A decision row is permanent and keyed by the address, so what is not one
// bare email address must never become a key. Every refusal lands before the
// transaction, so no database is needed to see it.
func TestASenderDecisionOnSomethingThatIsNotABareAddressIsRefusedNamingAddress(t *testing.T) {
	user := ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Create: true, Delete: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	store := NewSenderOverrideStore(nil)

	for name, address := range map[string]string{
		"not an email":          "not-an-email",
		"5000 characters":       strings.Repeat("a", 5000-len("@example.com")) + "@example.com",
		"over the cap":          strings.Repeat("a", maxIndexedAddressChars+1-len("@example.com")) + "@example.com",
		"two addresses":         "a@example.com, b@example.com",
		"a display name":        "Ada <ada@example.com>",
		"an angle-bracket form": "<ada@example.com>",
	} {
		_, err := store.Set(ctx, address, OverrideKeepOut)
		var invalid *InvalidOverrideError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: Set gave %v, want an InvalidOverrideError", name, err)
			continue
		}
		if field, _, _ := invalid.FieldFault(); field != fieldAddress {
			t.Errorf("%s: refusal names %q, want %q", name, field, fieldAddress)
		}
	}
}

// What is stored is what Remove and the verdict look up: one fold, so a
// decision that was accepted can always be found again.
func TestAnAcceptedSenderAddressFoldsTheWayTheLookupDoes(t *testing.T) {
	for _, address := range []string{"ada@example.com", "  Ada@Example.COM ", "ADA@EXAMPLE.COM"} {
		stored, ok := parseSingleAddress(address)
		if !ok {
			t.Fatalf("%q was refused", address)
		}
		if looked := normalizeEmail(address); stored != looked {
			t.Errorf("%q is stored as %q but looked up as %q", address, stored, looked)
		}
	}
}
