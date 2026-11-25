// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// readableCalendar answers every colleague's calendar as one a link may read.
type readableCalendar struct{ activities.SchedulingCalendar }

func (readableCalendar) Blocker(context.Context, ids.UserID, string) (crmcontracts.SchedulingProfileBlocker, error) {
	return crmcontracts.SchedulingProfileBlockerNone, nil
}

func (readableCalendar) List(context.Context, ids.UserID, string) ([]connector.CalendarOption, error) {
	return []connector.CalendarOption{{ID: "primary", Name: "Work", Writable: true, Primary: true}}, nil
}

// teamRequestMailEnv is Rep1's booking link naming Rep2, with the production
// store, writer and mail worker over a relay that counts.
type teamRequestMailEnv struct {
	e      *integration.Env
	store  *activities.Store
	relay  *countingMailer
	worker *notificationMailWorker
}

func setupTeamRequestMail(t *testing.T) *teamRequestMailEnv {
	t.Helper()
	e := integration.Setup(t)
	env := &teamRequestMailEnv{
		e:     e,
		store: activities.NewStore(e.DB()).WithSchedulingCalendar(readableCalendar{}).WithTeamRequestNotice(teamRequestNotice{}),
		relay: &countingMailer{},
	}
	env.worker = newNotificationMailWorker(e.Pool, NotificationMailConfig{Mailer: env.relay, PublicBaseURL: noticeMailOrigin}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	env.saveLink(t, env.e.Rep2, "Own link")
	return env
}

func (env *teamRequestMailEnv) as(user ids.UUID) context.Context {
	return env.e.As(user, []ids.UUID{env.e.Team1}, integration.SchedulerPerms)
}

// saveLink saves user's booking link with the given title and team.
func (env *teamRequestMailEnv) saveLink(t *testing.T, user ids.UUID, title string, team ...ids.UUID) {
	t.Helper()
	members := []crmcontracts.SchedulingTeamMember{}
	for _, member := range team {
		members = append(members, crmcontracts.SchedulingTeamMember{UserId: openapi_types.UUID(member)})
	}
	if _, err := env.store.SaveSchedulingProfile(env.as(user), crmcontracts.SchedulingProfile{
		Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: title, TeamMembers: &members,
	}); err != nil {
		t.Fatalf("saving %s's booking link: %v", user, err)
	}
}

func (env *teamRequestMailEnv) requestNotice(t *testing.T) ids.UUID {
	t.Helper()
	var notice ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT id FROM notice WHERE recipient_user_id = $1 AND kind = 'meeting_team_request'`, env.e.Rep2).Scan(&notice); err != nil {
		t.Fatalf("reading the team request notice: %v", err)
	}
	return notice
}

func (env *teamRequestMailEnv) memberAddress(t *testing.T) string {
	t.Helper()
	var address string
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT email FROM app_user WHERE id = $1`, env.e.Rep2).Scan(&address); err != nil {
		t.Fatalf("reading the member's address: %v", err)
	}
	return address
}

// The mail goes to the member and names the link as it stands when it leaves,
// not as it stood when the member was added.
func TestATeamRequestMailNamesTheLinkAsItStandsWhenItLeaves(t *testing.T) {
	env := setupTeamRequestMail(t)
	env.saveLink(t, env.e.Rep1, "Intro call", env.e.Rep2)
	env.saveLink(t, env.e.Rep1, "Renewal call", env.e.Rep2)
	job := &river.Job[SendNotificationEmailArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: notificationMailJobAttempts},
		Args:   SendNotificationEmailArgs{Workspace: env.e.WS, NoticeID: env.requestNotice(t).String()},
	}
	if err := env.worker.Work(context.Background(), job); err != nil {
		t.Fatalf("the notification mail worker refused the job: %v", err)
	}
	if env.relay.count() != 1 || env.relay.sends[0] != env.memberAddress(t) {
		t.Fatalf("the relay was handed %v, want one mail to the member", env.relay.sends)
	}
	body := env.relay.bodies[0]
	if !strings.Contains(body, "Renewal call") || strings.Contains(body, "Intro call") {
		t.Errorf("the mail does not name the link's current title:\n%s", body)
	}
	if !strings.Contains(body, noticeMailOrigin+"/#/settings/meetings") {
		t.Errorf("the mail does not open the meeting settings:\n%s", body)
	}
}

// A member taken off the link after the mail was claimed and before the relay
// is dialled is sent nothing.
func TestATeamRequestTakenBackAfterItsClaimIsNotSent(t *testing.T) {
	env := setupTeamRequestMail(t)
	env.saveLink(t, env.e.Rep1, "Intro call", env.e.Rep2)
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), env.e.WS),
		principal.Principal{Type: principal.PrincipalSystem, ID: notificationMailActor})
	attempt, claimed, err := env.worker.notices.ClaimEmailAttempt(ctx, env.requestNotice(t))
	if err != nil || !claimed {
		t.Fatalf("claiming the mail → claimed=%v, err=%v", claimed, err)
	}
	env.saveLink(t, env.e.Rep1, "Intro call")
	if err := env.worker.deliver(ctx, attempt, false); err != nil {
		t.Fatalf("delivering the claimed mail: %v", err)
	}
	if got := env.relay.count(); got != 0 {
		t.Fatalf("the relay was handed %d mails about a request taken back, want none", got)
	}
}

// A member named again after a removal gets their line back with the link's
// current title.
func TestATeamRequestOpenedAgainNamesTheCurrentTitle(t *testing.T) {
	env := setupTeamRequestMail(t)
	env.saveLink(t, env.e.Rep1, "Intro call", env.e.Rep2)
	env.saveLink(t, env.e.Rep1, "Intro call")
	env.saveLink(t, env.e.Rep1, "Renewal call", env.e.Rep2)
	var subject string
	var retracted bool
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT subject, retracted_at IS NOT NULL FROM notice WHERE id = $1`,
		env.requestNotice(t)).Scan(&subject, &retracted); err != nil {
		t.Fatalf("reading the notice: %v", err)
	}
	if retracted || !strings.Contains(subject, "Renewal call") {
		t.Fatalf("the reopened notice reads %q (retracted %v), want the current title", subject, retracted)
	}
}

// A send reserved against a team request is refused when the request moved
// after the payload was read.
func TestATeamRequestThatMovedAfterItsReadRefusesTheReservation(t *testing.T) {
	env := setupTeamRequestMail(t)
	env.saveLink(t, env.e.Rep1, "Intro call", env.e.Rep2)
	ctx := principal.WithWorkspaceID(context.Background(), env.e.WS)
	refs := sendRefs{teamRequests: []teamRequestRef{{notice: env.requestNotice(t), host: ids.From[ids.UserKind](env.e.Rep1), member: ids.From[ids.UserKind](env.e.Rep2)}}}
	var basis sendBasis
	if err := env.e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		basis, err = readSendBasis(ctx, tx, env.e.Rep2, nil, refs, clocktest.Now(t))
		return err
	}); err != nil || len(basis.rows) != 1 || len(basis.excluded) != 0 {
		t.Fatalf("reading the basis → %+v, %v; want the pending request held", basis, err)
	}
	// While the reservation holds, the host cannot change the link under it.
	if err := env.e.DB().Tx(ctx, func(tx pgx.Tx) error {
		if err := holdSendBasisTx(ctx, tx, basis, func() time.Time { return clocktest.Now(t) }); err != nil {
			return err
		}
		_, err := integration.OwnerConn(t).Exec(context.Background(),
			`SELECT 1 FROM booking_page WHERE host_user_id = $1 AND revoked_at IS NULL FOR UPDATE NOWAIT`, env.e.Rep1)
		if err == nil {
			t.Error("the link could be locked for a change while a send was reserved against it")
		}
		return nil
	}); err != nil {
		t.Fatalf("holding the unchanged request: %v", err)
	}
	env.saveLink(t, env.e.Rep1, "Renewal call", env.e.Rep2)
	err := env.e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return holdSendBasisTx(ctx, tx, basis, func() time.Time { return clocktest.Now(t) })
	})
	if !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("holding a request renamed since its read → %v, want %v", err, errSendBasisChanged)
	}
}
