// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// teamRequestNotice is the line, and the mail an approval would send, that a
// colleague named on a host's team link gets. There is one request per team
// link and member. A member taken off and named again gets the same line back
// unread. A member named on two team links gets a line for each.
type teamRequestNotice struct{}

func teamRequestKey(page ids.UUID, member ids.UserID) string {
	return notices.KindMeetingTeamRequest + ":" + page.String() + ":" + member.String()
}

func (teamRequestNotice) TeamRequestTx(ctx context.Context, tx pgx.Tx, request activities.TeamRequest) error {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("compose: a booking link named a colleague outside a bound workspace")
	}
	mail, err := callerTxJobs()
	if err != nil {
		return err
	}
	subject := systemtext.Sentence("booking.team_request.subject", systemtext.Text(request.HostName), systemtext.Text(request.Title))
	body := systemtext.Sentence("booking.team_request.body")
	line := notices.NewNotice{
		Recipient: request.Member, Kind: notices.KindMeetingTeamRequest,
		Subject: systemtext.Canonical(subject, textlang.English), Body: systemtext.Canonical(body, textlang.English),
		SystemText: &systemtext.Message{Version: 1, Subject: subject, Body: body},
		DedupeKey:  teamRequestKey(request.Page, request.Member),
	}
	return recordApprovalNotice(ctx, tx, notices.NewStore(nil).HandOverTx, mail, wsID, line)
}

func (teamRequestNotice) TeamRequestSettledTx(ctx context.Context, tx pgx.Tx, page ids.UUID, member ids.UserID) error {
	_, err := notices.NewStore(nil).Retract(ctx, tx, teamRequestKey(page, member))
	return err
}
