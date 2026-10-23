// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheBriefEndpointAnswersARefusalWithItsStatus(t *testing.T) {
	for name, tc := range map[string]struct {
		kind  principal.PrincipalType
		rooms *countingRooms
		want  int
	}{
		"an agent":           {principal.PrincipalAgent, &countingRooms{}, http.StatusForbidden},
		"a room out of view": {principal.PrincipalHuman, &countingRooms{roomErr: apperrors.ErrNotFound}, http.StatusNotFound},
	} {
		svc := NewService(nil, tc.rooms, func() time.Time { return clocktest.Now(t) })
		req := httptest.NewRequestWithContext(actor(tc.kind), http.MethodGet, "/v1/deal-rooms/x/brief", nil)
		rec := httptest.NewRecorder()

		NewHandlers(svc).GetDealRoomBrief(rec, req, crmcontracts.Id(ids.NewV7()))

		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", name, rec.Code, tc.want)
		}
	}
}
