// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyTheExactAddressIsTheSeatsAddress(t *testing.T) {
	seat := ids.NewV7()
	set := SeatAddresses{
		proven:   map[string]struct{}{"jane.doe@gmail.com": {}},
		logins:   map[string]struct{}{"jane.doe@gmail.com": {}},
		declared: map[string]map[ids.UUID]struct{}{},
	}
	for _, c := range []struct {
		address string
		want    bool
	}{
		{"jane.doe@gmail.com", true},
		{" Jane.Doe@GMAIL.com ", true},
		{"jane.doe+news@gmail.com", false},
		{"janedoe@gmail.com", false},
		{"j.a.n.e.doe@gmail.com", false},
		{"jane.doe@mail.gmail.com", false},
	} {
		if got := set.Covers(c.address, seat); got != c.want {
			t.Errorf("Covers(%q) = %v, want %v", c.address, got, c.want)
		}
		if got := set.IsLogin(c.address); got != c.want {
			t.Errorf("IsLogin(%q) = %v, want %v", c.address, got, c.want)
		}
	}
}
