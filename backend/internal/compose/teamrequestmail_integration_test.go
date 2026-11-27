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
	"github.com/margince/margince/backend/internal/modules/notices"
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

// teamRequestMailEnv is Rep1's team link naming Rep2, with the production
// store, writer and mail worker over a relay that counts. Rep3 stays on the
// link throughout, so Rep2 can be taken off it and named again.
type teamRequestMailEnv struct {
	e      *integration.Env
	store  *activities.Store
	relay  *countingMailer
	worker *notificationMailWorker
	link   ids.UUID
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
	for _, member := range []ids.UUID{env.e.Rep2, env.e.Rep3} {
		if _, err := env.store.SaveSchedulingProfile(env.as(member), crmcontracts.SchedulingProfile{
			Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Own link",
		}); err != nil {
			t.Fatalf("saving %s's own booking link: %v", member, err)
		}
	}
	return env
}

func (env *teamRequestMailEnv) as(user ids.UUID) context.Context {
	return env.e.As(user, []ids.UUID{env.e.Team1}, integration.SchedulerPerms)
}

// saveLink saves Rep1's team link with the given title, naming Rep3 and team.
// The first save makes the link.
func (env *teamRequestMailEnv) saveLink(t *testing.T, title string, team ...ids.UUID) {
	t.Helper()
	members := []crmcontracts.SchedulingTeamMember{{UserId: openapi_types.UUID(env.e.Rep3)}}
	for _, member := range team {
		members = append(members, crmcontracts.SchedulingTeamMember{UserId: openapi_types.UUID(member)})
	}
	request := crmcontracts.SchedulingTeamLinkRequest{Title: title, TeamMembers: members}
	if env.link == ids.Nil {
		link, err := env.store.CreateSchedulingTeamLink(env.as(env.e.Rep1), request)
		if err != nil {
			t.Fatalf("making Rep1's team link: %v", err)
		}
		env.link = ids.UUID(link.Id)
		return
	}
	if _, err := env.store.UpdateSchedulingTeamLink(env.as(env.e.Rep1), env.link, request); err != nil {
		t.Fatalf("saving Rep1's team link: %v", err)
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
	t.Parallel()
	env := setupTeamRequestMail(t)
	env.saveLink(t, "Intro call", env.e.Rep2)
	env.saveLink(t, "Renewal call", env.e.Rep2)
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
	t.Parallel()
	env := setupTeamRequestMail(t)
	env.saveLink(t, "Intro call", env.e.Rep2)
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), env.e.WS),
		principal.Principal{Type: principal.PrincipalSystem, ID: notificationMailActor})
	attempt, claimed, err := env.worker.notices.ClaimEmailAttempt(ctx, env.requestNotice(t))
	if err != nil || !claimed {
		t.Fatalf("claiming the mail → claimed=%v, err=%v", claimed, err)
	}
	env.saveLink(t, "Intro call")
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
	t.Parallel()
	env := setupTeamRequestMail(t)
	env.saveLink(t, "Intro call", env.e.Rep2)
	env.saveLink(t, "Intro call")
	env.saveLink(t, "Renewal call", env.e.Rep2)
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
	t.Parallel()
	env := setupTeamRequestMail(t)
	env.saveLink(t, "Intro call", env.e.Rep2)
	ctx := principal.WithWorkspaceID(context.Background(), env.e.WS)
	now := func() time.Time { return clocktest.Now(t) }
	refs := sendRefs{teamRequests: []teamRequestRef{{notice: env.requestNotice(t), page: env.link, member: ids.From[ids.UserKind](env.e.Rep2)}}}
	var basis sendBasis
	if err := env.e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		basis, err = readSendBasis(ctx, tx, env.e.Rep2, nil, refs, now())
		return err
	}); err != nil || len(basis.rows) != 1 || len(basis.excluded) != 0 {
		t.Fatalf("reading the basis → %+v, %v; want the pending request held", basis, err)
	}
	// While the reservation holds, the host cannot change the link under it.
	if err := env.e.DB().Tx(ctx, func(tx pgx.Tx) error {
		if err := holdSendBasisTx(ctx, tx, basis, now); err != nil {
			return err
		}
		_, err := integration.OwnerConn(t).Exec(context.Background(),
			`SELECT 1 FROM booking_page WHERE id = $1 FOR UPDATE NOWAIT`, env.link)
		if err == nil {
			t.Error("the link could be locked for a change while a send was reserved against it")
		}
		return nil
	}); err != nil {
		t.Fatalf("holding the unchanged request: %v", err)
	}
	env.saveLink(t, "Renewal call", env.e.Rep2)
	err := env.e.DB().Tx(ctx, func(tx pgx.Tx) error { return holdSendBasisTx(ctx, tx, basis, now) })
	if !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("holding a request renamed since its read → %v, want %v", err, errSendBasisChanged)
	}
}

// noticeOn is Rep2's team request notice on one team link.
func (env *teamRequestMailEnv) noticeOn(t *testing.T, link ids.UUID) ids.UUID {
	t.Helper()
	var notice ids.UUID
	key := notices.TeamRequestNoticeKey(link, ids.From[ids.UserKind](env.e.Rep2))
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT id FROM notice WHERE dedupe_key = $1`, key).Scan(&notice); err != nil {
		t.Fatalf("reading the team request notice on %s: %v", link, err)
	}
	return notice
}

// Each mail is checked against its own team link. A colleague taken off one of
// a host's two links hears nothing more about it, and is still told of the other.
func TestEachTeamRequestMailReadsItsOwnTeamLink(t *testing.T) {
	t.Parallel()
	env := setupTeamRequestMail(t)
	env.saveLink(t, "Intro call", env.e.Rep2)
	first := env.link
	second, err := env.store.CreateSchedulingTeamLink(env.as(env.e.Rep1), crmcontracts.SchedulingTeamLinkRequest{
		Title: "Renewal call", TeamMembers: []crmcontracts.SchedulingTeamMember{{UserId: openapi_types.UUID(env.e.Rep2)}},
	})
	if err != nil {
		t.Fatalf("making Rep1's second team link: %v", err)
	}
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), env.e.WS),
		principal.Principal{Type: principal.PrincipalSystem, ID: notificationMailActor})
	claim := func(notice ids.UUID) notices.EmailAttempt {
		attempt, claimed, err := env.worker.notices.ClaimEmailAttempt(ctx, notice)
		if err != nil || !claimed {
			t.Fatalf("claiming the mail → claimed=%v, err=%v", claimed, err)
		}
		return attempt
	}
	onFirst, onSecond := claim(env.noticeOn(t, first)), claim(env.noticeOn(t, ids.UUID(second.Id)))

	env.saveLink(t, "Intro call")
	for _, attempt := range []notices.EmailAttempt{onFirst, onSecond} {
		if err := env.worker.deliver(ctx, attempt, false); err != nil {
			t.Fatalf("delivering a claimed mail: %v", err)
		}
	}
	if env.relay.count() != 1 {
		t.Fatalf("the relay was handed %d mails, want only the one for the link Rep2 is still on", env.relay.count())
	}
	if body := env.relay.bodies[0]; !strings.Contains(body, "Renewal call") || strings.Contains(body, "Intro call") {
		t.Errorf("the mail does not name the link it is about:\n%s", body)
	}
}
