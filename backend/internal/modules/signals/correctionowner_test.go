// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCorrectionOwnerNamesARealMemberOnly(t *testing.T) {
	owner, actor := ids.UUID{1}, ids.UUID{2}
	zero := ids.UUID{}
	cases := []struct {
		name  string
		owner *ids.UUID
		actor ids.UUID
		want  ids.UUID
		ok    bool
	}{
		{"owner wins", &owner, actor, owner, true},
		{"no owner names the actor", nil, actor, actor, true},
		{"zero owner names the actor", &zero, actor, actor, true},
		{"no owner and no member id", nil, zero, zero, false},
	}
	for _, c := range cases {
		got, ok := correctionOwner(c.owner, c.actor)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %v,%v want %v,%v", c.name, got, ok, c.want, c.ok)
		}
	}
}
