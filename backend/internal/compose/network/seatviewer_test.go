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
		{"agent on a passport", principal.Principal{Type: principal.PrincipalAgent, UserID: user, PassportID: passport, OnBehalfOf: user}, user},
		{"agent whose user differs from its principal", principal.Principal{Type: principal.PrincipalAgent, UserID: other, PassportID: passport, OnBehalfOf: user}, ids.Nil},
		{"agent without a user id", principal.Principal{Type: principal.PrincipalAgent, PassportID: passport, OnBehalfOf: user}, ids.Nil},
		{"passportless agent", principal.Principal{Type: principal.PrincipalAgent, UserID: user, OnBehalfOf: user}, ids.Nil},
		{"connector", principal.Principal{Type: principal.PrincipalConnector, UserID: user, OnBehalfOf: user}, ids.Nil},
		{"system", principal.Principal{Type: principal.PrincipalSystem}, ids.Nil},
	}
	for _, c := range cases {
		if got := seatViewer(c.p); got != c.want {
			t.Errorf("%s: seatViewer = %v, want %v", c.name, got, c.want)
		}
	}
}
