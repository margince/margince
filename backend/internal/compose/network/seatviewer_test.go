// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSeatViewerAsksAnAgentAsItsUser(t *testing.T) {
	user, passport, other := ids.NewV7(), ids.NewV7(), ids.NewV7()
	cases := []struct {
		name string
		p    principal.Principal
		want ids.UUID
	}{
		{"human", principal.Principal{Type: principal.PrincipalHuman, UserID: user}, user},
		{"agent on a passport without a user id", principal.Principal{Type: principal.PrincipalAgent, PassportID: passport, OnBehalfOf: user}, user},
		{"agent on a passport", principal.Principal{Type: principal.PrincipalAgent, UserID: other, PassportID: passport, OnBehalfOf: user}, user},
		{"passportless agent stays proof-only", principal.Principal{Type: principal.PrincipalAgent, OnBehalfOf: user}, ids.Nil},
	}
	for _, c := range cases {
		if got := seatViewer(c.p); got != c.want {
			t.Errorf("%s: seatViewer = %v, want %v", c.name, got, c.want)
		}
	}
}
