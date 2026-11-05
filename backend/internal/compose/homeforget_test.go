// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAWriteEndsTheCachedHomeOfEachReaderItWasMadeForOnce(t *testing.T) {
	human, other := ids.NewV7(), ids.NewV7()
	cases := map[string]struct {
		method string
		actor  principal.Principal
		want   []ids.UUID
	}{
		"a human's write":                    {"POST", principal.Principal{Type: principal.PrincipalHuman, UserID: human}, []ids.UUID{human}},
		"an agent acting as its human":       {"PUT", principal.Principal{Type: principal.PrincipalAgent, UserID: human, OnBehalfOf: human}, []ids.UUID{human}},
		"an agent seat acting for its human": {"PATCH", principal.Principal{Type: principal.PrincipalAgent, UserID: other, OnBehalfOf: human}, []ids.UUID{other, human}},
		"a read":                             {"GET", principal.Principal{Type: principal.PrincipalHuman, UserID: human}, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "/v1/contacts", nil)
			r = r.WithContext(principal.WithActor(r.Context(), c.actor))
			if got := writersOf(r); !slices.Equal(got, c.want) {
				t.Errorf("writers %v, want %v", got, c.want)
			}
		})
	}
}
