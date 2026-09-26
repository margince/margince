// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func resolveCapturedInvitation(ctx context.Context, tx pgx.Tx, key connector.NaturalKey, raw []byte) (ids.ActivityID, bool, error) {
	var request string
	var err error
	switch key.SourceSystem {
	case "gcal":
		request, err = gcal.InvitationRequest(key.SourceID, raw)
	case "graphcal":
		request, err = graphcal.InvitationRequest(key.SourceID, raw)
	default:
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, err
	}
	return activities.ResolveCalendarInvitation(ctx, tx, key.SourceSystem, key.SourceID, request)
}
