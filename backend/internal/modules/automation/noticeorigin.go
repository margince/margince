// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func noticeOrigin(ev workflow.Event) *crmcontracts.NoticeOrigin {
	if ev.ID.IsZero() || ev.Actor.Type == "" || ev.Actor.ID == "" || ev.OccurredAt.IsZero() {
		return nil
	}
	origin := &crmcontracts.NoticeOrigin{EventId: openapi_types.UUID(ev.ID), ActorType: ev.Actor.Type, ActorId: ev.Actor.ID, OccurredAt: ev.OccurredAt}
	if ev.Actor.OnBehalfOf != nil {
		id := openapi_types.UUID(*ev.Actor.OnBehalfOf)
		origin.OnBehalfOf = &id
	}
	return origin
}
