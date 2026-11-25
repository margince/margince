// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMergeRefusalsNameOnlyWhatNoConfirmationCanFix(t *testing.T) {
	actor := Identity{UserID: ids.UserID{UUID: ids.UUID{1}}, Roles: []string{roleAdmin}}
	other := ids.UUID{2}
	cases := []struct {
		name   string
		target memberTarget
		want   []string
	}{
		{"an unused member", memberTarget{row: userRow{ID: other}}, nil},
		{"a used member waits for confirmation instead", memberTarget{row: userRow{ID: other, Usage: memberUsage{PasswordSet: true, SignedIn: true, ProviderLinked: true}}}, nil},
		{"the sole administrator, also used", memberTarget{soleAdmin: true, row: userRow{ID: other, Usage: memberUsage{PasswordSet: true}}}, []string{reasonSoleAdmin}},
		{"the caller", memberTarget{row: userRow{ID: actor.UserID.UUID, Usage: memberUsage{PasswordSet: true}}}, []string{reasonSelf}},
		{"an agent seat", memberTarget{row: userRow{ID: other, IsAgent: true}}, []string{reasonAgentSeat}},
	}
	for _, c := range cases {
		if got := mergeRefusals(actor, c.target); !slices.Equal(got, c.want) {
			t.Errorf("%s: refusals = %v, want %v", c.name, got, c.want)
		}
	}
}
