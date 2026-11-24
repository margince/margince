// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
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

func (teamRequestNotice) TeamRequestTx(ctx context.Context, tx pgx.Tx, request activities.TeamRequest) error {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("compose: a booking link named a colleague outside a bound workspace")
	}
	mail, err := callerTxJobs()
	if err != nil {
		return err
	}
	return recordApprovalNotice(ctx, tx, notices.NewStore(nil).HandOverTx, mail, wsID, teamRequestLine(request))
}

func (teamRequestNotice) TeamRequestSettledTx(ctx context.Context, tx pgx.Tx, page ids.UUID, member ids.UserID) error {
	_, err := notices.NewStore(nil).Retract(ctx, tx, notices.TeamRequestNoticeKey(page, member))
	return err
}

func (teamRequestNotice) TeamRequestRetitledTx(ctx context.Context, tx pgx.Tx, request activities.TeamRequest) error {
	return notices.NewStore(nil).RewordLiveTx(ctx, tx, teamRequestLine(request))
}

// teamRequestLine is the request in words: when it is written, and again from
// the link as it stands when its mail leaves.
func teamRequestLine(request activities.TeamRequest) notices.NewNotice {
	subject := systemtext.Sentence("booking.team_request.subject", systemtext.Text(request.HostName), systemtext.Text(request.Title))
	body := systemtext.Sentence("booking.team_request.body")
	return notices.NewNotice{
		Recipient: request.Member, Kind: notices.KindMeetingTeamRequest,
		Subject: systemtext.Canonical(subject, textlang.English), Body: systemtext.Canonical(body, textlang.English),
		SystemText: &systemtext.Message{Version: 1, Subject: subject, Body: body},
		DedupeKey:  notices.TeamRequestNoticeKey(request.Page, request.Member),
	}
}

// refreshTeamRequest rewrites a claimed team request from its link as it stands.
// A request no longer pending is not current, and its mail is not sent.
func refreshTeamRequest(ctx context.Context, pool *pgxpool.Pool, attempt notices.EmailAttempt) (notices.EmailAttempt, bool, error) {
	var standing activities.TeamRequestStanding
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		standing, err = activities.ReadTeamRequestTx(ctx, tx, *attempt.TeamRequestPage, attempt.Recipient)
		return err
	})
	if err != nil || !standing.Pending {
		return attempt, false, err
	}
	line := teamRequestLine(standing.Request)
	attempt.Notice.Subject, attempt.Notice.Body, attempt.Notice.SystemText = line.Subject, line.Body, line.SystemText
	return attempt, true, nil
}

// teamRequestBasis names a team request a delayed send rests on. Its id is the
// notice, its key the team link and member, its version the request's fingerprint.
const teamRequestBasis = "booking_page_team_request"

type teamRequestRef struct {
	notice, page ids.UUID
	member       ids.UserID
}

// teamRequestDependencies reads each request a payload names: one still pending
// is held at its fingerprint, any other is excluded.
func teamRequestDependencies(ctx context.Context, tx pgx.Tx, refs []teamRequestRef, basis *sendBasis) error {
	for _, ref := range refs {
		standing, err := activities.ReadTeamRequestTx(ctx, tx, ref.page, ref.member)
		if err != nil {
			return err
		}
		if !standing.Pending {
			basis.excluded[ref.notice] = true
			continue
		}
		basis.rows = append(basis.rows, basisRow{table: teamRequestBasis, id: ref.notice, key: ref.page.String() + ":" + ref.member.String(), version: standing.Fingerprint})
	}
	return nil
}

// holdTeamRequest holds the link a basis row names and refuses a request that
// moved since it was read.
func holdTeamRequest(ctx context.Context, tx pgx.Tx, row basisRow) error {
	page, member, ok := strings.Cut(row.key, ":")
	pageID, pageErr := ids.Parse(page)
	memberID, memberErr := ids.Parse(member)
	if !ok || pageErr != nil || memberErr != nil {
		return errors.New("send: a team request basis names no team link and member")
	}
	standing, err := activities.HoldTeamRequestTx(ctx, tx, pageID, ids.From[ids.UserKind](memberID))
	if err != nil {
		return err
	}
	if !standing.Pending || standing.Fingerprint != row.version {
		return errSendBasisChanged
	}
	return nil
}
