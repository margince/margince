// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	pushSecret = "whsec_cHVzaC1zZWNyZXQtZm9yLXRlc3Rz"
	pushTeam   = "team-ours"
)

var pushNow = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

type fakeEventBots struct {
	bot    capture.LiveBot
	secret string
	err    error
}

func (b fakeEventBots) LiveBotEvents(context.Context, string) (capture.LiveBot, string, error) {
	return b.bot, b.secret, b.err
}

type fakeEventLinks struct {
	links    []chatlinks.Link
	gone     []ids.UUID
	failGone int
}

func (l *fakeEventLinks) PullLinks(_ context.Context, connection ids.UUID) ([]chatlinks.Link, error) {
	var out []chatlinks.Link
	for _, link := range l.links {
		if link.ConnectionID == connection {
			out = append(out, link)
		}
	}
	return out, nil
}

func (l *fakeEventLinks) MarkChannelGone(_ context.Context, id ids.UUID) error {
	if l.failGone > 0 {
		l.failGone--
		return errors.New("database down")
	}
	l.gone = append(l.gone, id)
	return nil
}

type fakeReplays map[string]bool

func (r fakeReplays) FirstUse(_ context.Context, key string, _ time.Duration) (bool, error) {
	if r[key] {
		return false, nil
	}
	r[key] = true
	return true, nil
}

func (r fakeReplays) Release(_ context.Context, key string) error {
	delete(r, key)
	return nil
}

type fakeMarks map[string]time.Time

// brokenMarks is a mark store that cannot be read.
type brokenMarks struct{}

func (brokenMarks) Mark(context.Context, string, time.Time, time.Duration) error { return nil }
func (brokenMarks) Last(context.Context, string) (time.Time, error) {
	return time.Time{}, errors.New("redis down")
}

func (m fakeMarks) Mark(_ context.Context, key string, at time.Time, _ time.Duration) error {
	m[key] = at
	return nil
}

func (m fakeMarks) Last(_ context.Context, key string) (time.Time, error) { return m[key], nil }

type pushEnv struct {
	receiver  *Receiver
	links     *fakeEventLinks
	marks     fakeMarks
	replays   fakeReplays
	pulls     []ids.UUID
	failPulls int
	conn      ids.UUID
	link      chatlinks.Link
	now       time.Time
}

func newPushEnv(t *testing.T) *pushEnv {
	t.Helper()
	e := &pushEnv{conn: ids.NewV7(), marks: fakeMarks{}, replays: fakeReplays{}, now: pushNow}
	e.link = chatlinks.Link{ID: ids.NewV7(), ConnectionID: e.conn, ChannelSlug: "deal-acme", Status: chatlinks.StatusActive}
	e.links = &fakeEventLinks{links: []chatlinks.Link{
		e.link,
		{ID: ids.NewV7(), ConnectionID: ids.NewV7(), ChannelSlug: "deal-other", Status: chatlinks.StatusActive},
	}}
	e.receiver = &Receiver{
		Bots:  fakeEventBots{bot: capture.LiveBot{ConnectionID: e.conn, TeamID: pushTeam}, secret: pushSecret},
		Links: e.links, Replays: e.replays, Marks: e.marks, Now: func() time.Time { return e.now }, Log: slog.Default(),
		Pull: func(_ context.Context, _, connection ids.UUID) error {
			if e.failPulls > 0 {
				e.failPulls--
				return errors.New("queue down")
			}
			e.pulls = append(e.pulls, connection)
			return nil
		},
	}
	return e
}

// signed builds a delivery as Dispact sends it, signed at the given time.
func signed(t *testing.T, secret, id string, at time.Time, body string) chatlinks.Delivery {
	t.Helper()
	sig, err := webhooks.Sign(secret, id, at.Unix(), []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return chatlinks.Delivery{ID: id, Timestamp: strconv.FormatInt(at.Unix(), 10), Signature: sig, Body: []byte(body)}
}

func notice(kind, team, slug string) string {
	return fmt.Sprintf(`{"type":%q,"workspace_id":%q,"channel_slug":%q,"seq":42,"event_id":"ev1","ts":1}`, kind, team, slug)
}

func (e *pushEnv) receive(d chatlinks.Delivery) (crmcontracts.DispactChannelEventReceipt, error) {
	return e.receiver.Receive(principal.WithWorkspaceID(context.Background(), ids.NewV7()), d)
}

func statusOf(r crmcontracts.DispactChannelEventReceipt) string {
	if r.Status == nil {
		return ""
	}
	return string(*r.Status)
}

func TestASignedNoticeQueuesThePullAndMarksThePush(t *testing.T) {
	e := newPushEnv(t)
	got, err := e.receive(signed(t, pushSecret, "msg_1", pushNow, notice("message.created", pushTeam, "deal-acme")))
	if err != nil || statusOf(got) != "pull_queued" {
		t.Fatalf("a signed notice → %+v, %v; want pull_queued", got, err)
	}
	if len(e.pulls) != 1 || e.pulls[0] != e.conn {
		t.Errorf("pulls queued %v; want one for the bot's connection", e.pulls)
	}
	if !e.marks[noticeKey(e.conn)].Equal(pushNow) {
		t.Errorf("the notice was not marked at %v: %v", pushNow, e.marks)
	}
}

func TestANoticeThatFailsAnyCheckIsRejectedAndPullsNothing(t *testing.T) {
	body := notice("message.created", pushTeam, "deal-acme")
	for _, tc := range []struct {
		name     string
		delivery func(t *testing.T) chatlinks.Delivery
	}{
		{"another secret", func(t *testing.T) chatlinks.Delivery {
			return signed(t, "whsec_YW5vdGhlci1zZWNyZXQ=", "msg_1", pushNow, body)
		}},
		{"a changed body", func(t *testing.T) chatlinks.Delivery {
			d := signed(t, pushSecret, "msg_1", pushNow, body)
			d.Body = []byte(notice("message.created", pushTeam, "deal-other"))
			return d
		}},
		{"a timestamp six minutes old", func(t *testing.T) chatlinks.Delivery {
			return signed(t, pushSecret, "msg_1", pushNow.Add(-6*time.Minute), body)
		}},
		{"a timestamp six minutes ahead", func(t *testing.T) chatlinks.Delivery {
			return signed(t, pushSecret, "msg_1", pushNow.Add(6*time.Minute), body)
		}},
		{"no signature", func(t *testing.T) chatlinks.Delivery {
			d := signed(t, pushSecret, "msg_1", pushNow, body)
			d.Signature = ""
			return d
		}},
		{"another team", func(t *testing.T) chatlinks.Delivery {
			return signed(t, pushSecret, "msg_1", pushNow, notice("message.created", "team-theirs", "deal-acme"))
		}},
	} {
		e := newPushEnv(t)
		if _, err := e.receive(tc.delivery(t)); !errors.Is(err, chatlinks.ErrEventRejected) {
			t.Errorf("%s: err = %v; want ErrEventRejected", tc.name, err)
		}
		if len(e.pulls) != 0 || len(e.marks) != 0 {
			t.Errorf("%s: a rejected notice queued %d pull(s) and marked %v", tc.name, len(e.pulls), e.marks)
		}
	}
}

func TestOneValidSignatureAmongSeveralIsEnough(t *testing.T) {
	e := newPushEnv(t)
	d := signed(t, pushSecret, "msg_1", pushNow, notice("thread.replied", pushTeam, "deal-acme"))
	d.Signature = "v1,b2xkLXNlY3JldC1zaWduYXR1cmU= " + d.Signature
	if got, err := e.receive(d); err != nil || statusOf(got) != "pull_queued" {
		t.Errorf("a rotated secret's entry beside the valid one → %+v, %v; want pull_queued", got, err)
	}
}

func TestAReplayedNoticeIDIsRefused(t *testing.T) {
	e := newPushEnv(t)
	d := signed(t, pushSecret, "msg_1", pushNow, notice("message.created", pushTeam, "deal-acme"))
	if _, err := e.receive(d); err != nil {
		t.Fatal(err)
	}
	if _, err := e.receive(d); !errors.Is(err, chatlinks.ErrEventReplayed) {
		t.Errorf("the same webhook-id a second time: %v; want ErrEventReplayed", err)
	}
	if len(e.pulls) != 1 {
		t.Errorf("a replay queued a second pull: %d", len(e.pulls))
	}
}

func TestTheURLCheckEchoesItsChallengeOnlyWhenSigned(t *testing.T) {
	e := newPushEnv(t)
	body := `{"type":"url_verification","challenge":"c-123"}`
	got, err := e.receive(signed(t, pushSecret, "msg_check", pushNow, body))
	if err != nil || got.Challenge == nil || *got.Challenge != "c-123" || got.Status != nil {
		t.Fatalf("a signed URL check → %+v, %v; want only the challenge c-123 back", got, err)
	}
	unsigned := chatlinks.Delivery{ID: "msg_check2", Timestamp: strconv.FormatInt(pushNow.Unix(), 10), Body: []byte(body)}
	if _, err := e.receive(unsigned); !errors.Is(err, chatlinks.ErrEventRejected) {
		t.Errorf("an unsigned URL check: %v; want ErrEventRejected", err)
	}
}

func TestANoticeForAChannelNoLinkHasChangesNothing(t *testing.T) {
	e := newPushEnv(t)
	for i, slug := range []string{"deal-unknown", "deal-other"} {
		d := signed(t, pushSecret, fmt.Sprintf("msg_%d", i), pushNow, notice("message.created", pushTeam, slug))
		if got, err := e.receive(d); err != nil || statusOf(got) != "ignored" {
			t.Errorf("a notice for %s → %+v, %v; want ignored", slug, got, err)
		}
	}
	if len(e.pulls) != 0 || len(e.links.gone) != 0 {
		t.Errorf("a notice for no link of this bot queued %d pull(s) and paused %v", len(e.pulls), e.links.gone)
	}
}

func TestAnArchivedChannelPausesItsLink(t *testing.T) {
	e := newPushEnv(t)
	got, err := e.receive(signed(t, pushSecret, "msg_1", pushNow, notice("channel.archived", pushTeam, "deal-acme")))
	if err != nil || statusOf(got) != "link_paused" {
		t.Fatalf("channel.archived → %+v, %v; want link_paused", got, err)
	}
	if len(e.links.gone) != 1 || e.links.gone[0] != e.link.ID || len(e.pulls) != 0 {
		t.Errorf("paused %v and pulled %d; want the link %s paused and no pull", e.links.gone, len(e.pulls), e.link.ID)
	}
}

func TestWithNoEventsSecretEveryNoticeIsRefused(t *testing.T) {
	for name, err := range map[string]error{"no secret": capture.ErrNoEventsSecret, "no bot": capture.ErrNoConnection} {
		e := newPushEnv(t)
		e.receiver.Bots = fakeEventBots{err: err}
		d := signed(t, pushSecret, "msg_1", pushNow, notice("message.created", pushTeam, "deal-acme"))
		if _, got := e.receive(d); !errors.Is(got, chatlinks.ErrEventRejected) {
			t.Errorf("%s: err = %v; want the same ErrEventRejected a bad signature gets", name, got)
		}
		if len(e.pulls) != 0 {
			t.Errorf("%s: %d pull(s) queued", name, len(e.pulls))
		}
	}
}

func TestTheSweepSlowsWhilePushWorksAndPollsWhenItStops(t *testing.T) {
	conn := ids.NewV7()
	marks := fakeMarks{}
	now := pushNow
	sweeper := Sweeper{Marks: marks, Now: func() time.Time { return now }, Log: slog.Default()}
	ctx := context.Background()
	tick := func() bool {
		due := sweeper.Due(ctx, conn)
		if due {
			sweeper.Swept(ctx, conn)
		}
		now = now.Add(time.Minute)
		return due
	}
	twoTicks := func() [2]bool { return [2]bool{tick(), tick()} }

	if twoTicks() != [2]bool{true, true} {
		t.Fatal("with no notice the sweep must read every minute")
	}
	marks[noticeKey(conn)] = now
	pulls := 0
	for range 60 {
		if tick() {
			pulls++
		}
		marks[noticeKey(conn)] = now
	}
	if pulls != 1 {
		t.Errorf("while notices kept coming the sweep read %d times in an hour; want once", pulls)
	}
	now = now.Add(PushFresh)
	if twoTicks() != [2]bool{true, true} {
		t.Error("an hour after the last notice the sweep must read every minute again")
	}
}

type closedChats struct{}

func (closedChats) Open(context.Context) (chatlinks.Session, error) {
	return chatlinks.Session{}, chatlinks.ErrBotNotConnected
}

type pullArgs struct{ conn ids.UUID }

func (pullArgs) Kind() string { return "chat_pull" }

func (pullArgs) WorkspaceID() ids.UUID { return ids.Nil }

func (a pullArgs) Connection() ids.UUID { return a.conn }

func bindAny(ctx context.Context, _ jobs.WorkspaceScoped) (context.Context, error) { return ctx, nil }

func TestANoticeDuringAPullRunsItOnceMore(t *testing.T) {
	conn := ids.NewV7()
	marks := fakeMarks{}
	worker := &PullWorker[pullArgs]{
		Bind: bindAny, Marks: marks,
		Puller: &Puller{Chats: closedChats{}, Now: func() time.Time { return pushNow }},
	}
	job := &river.Job[pullArgs]{Args: pullArgs{conn: conn}}

	marks[noticeKey(conn)] = pushNow.Add(-time.Second)
	if err := worker.Work(context.Background(), job); err != nil {
		t.Errorf("a notice from before the pull: %v; want the pull to end", err)
	}
	marks[noticeKey(conn)] = pushNow.Add(time.Second)
	var snooze *river.JobSnoozeError
	if err := worker.Work(context.Background(), job); !errors.As(err, &snooze) {
		t.Errorf("a notice that came while the pull ran: %v; want the pull to run again", err)
	}
}

func TestANoticeWhoseActionFailedIsTakenOnRetry(t *testing.T) {
	for _, tc := range []struct {
		kind, want string
		fail       func(e *pushEnv)
	}{
		{"message.created", "pull_queued", func(e *pushEnv) { e.failPulls = 1 }},
		{"channel.archived", "link_paused", func(e *pushEnv) { e.links.failGone = 1 }},
	} {
		e := newPushEnv(t)
		tc.fail(e)
		d := signed(t, pushSecret, "msg_1", pushNow, notice(tc.kind, pushTeam, "deal-acme"))
		if _, err := e.receive(d); err == nil || errors.Is(err, chatlinks.ErrEventReplayed) {
			t.Fatalf("%s: the failed action answered %v; want its own error", tc.kind, err)
		}
		if got, err := e.receive(d); err != nil || statusOf(got) != tc.want {
			t.Errorf("%s: Dispact's retry under the same id → %+v, %v; want %s", tc.kind, got, err, tc.want)
		}
		if _, err := e.receive(d); !errors.Is(err, chatlinks.ErrEventReplayed) {
			t.Errorf("%s: a replay after the success: %v; want ErrEventReplayed", tc.kind, err)
		}
	}
}

func TestANoticeOfUnknownShapeIsRefusedBeforeItCounts(t *testing.T) {
	for name, body := range map[string]string{
		"an unknown type":   notice("message.deleted", pushTeam, "deal-acme"),
		"no type":           `{"workspace_id":"team-ours","channel_slug":"deal-acme"}`,
		"no channel":        notice("message.created", pushTeam, ""),
		"a check, no value": `{"type":"url_verification"}`,
	} {
		e := newPushEnv(t)
		if _, err := e.receive(signed(t, pushSecret, "msg_1", pushNow, body)); !errors.Is(err, chatlinks.ErrEventMalformed) {
			t.Errorf("%s: err = %v; want ErrEventMalformed", name, err)
		}
		if len(e.replays) != 0 || len(e.marks) != 0 || len(e.pulls) != 0 {
			t.Errorf("%s: a refused notice spent %v, marked %v and pulled %d", name, e.replays, e.marks, len(e.pulls))
		}
	}
}

func TestAPullWhoseMarksCannotBeReadRunsOnceMore(t *testing.T) {
	worker := &PullWorker[pullArgs]{
		Bind: bindAny, Marks: brokenMarks{},
		Puller: &Puller{Chats: closedChats{}, Now: func() time.Time { return pushNow }},
	}
	var snooze *river.JobSnoozeError
	if err := worker.Work(context.Background(), &river.Job[pullArgs]{Args: pullArgs{conn: ids.NewV7()}}); !errors.As(err, &snooze) {
		t.Errorf("an unreadable notice mark: %v; want the pull to run again", err)
	}
}

func TestANoticeInTheMillisecondThePullStartedRunsItOnceMore(t *testing.T) {
	conn := ids.NewV7()
	start := pushNow.Add(400 * time.Microsecond)
	// The store keeps milliseconds, so a notice 300µs after the start reads back as pushNow.
	marks := fakeMarks{noticeKey(conn): time.UnixMilli(start.Add(300 * time.Microsecond).UnixMilli())}
	worker := &PullWorker[pullArgs]{
		Bind: bindAny, Marks: marks,
		Puller: &Puller{Chats: closedChats{}, Now: func() time.Time { return start }},
	}
	var snooze *river.JobSnoozeError
	if err := worker.Work(context.Background(), &river.Job[pullArgs]{Args: pullArgs{conn: conn}}); !errors.As(err, &snooze) {
		t.Errorf("a notice in the start's millisecond: %v; want the pull to run again", err)
	}
}
