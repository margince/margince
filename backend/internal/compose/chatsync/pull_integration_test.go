// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact/dispacttest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// linkedAt is when the test link was made; talk before it is history.
var linkedAt = time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

// scriptedSummary answers like the episode site: talk about lunch does not
// matter, anything else does and is summarised in fixed words.
type scriptedSummary struct{ calls int }

func (s *scriptedSummary) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.calls++
	if strings.Contains(req.Messages[0].Content, "lunch") {
		return model.Response{Text: `{"relevant":false,"reason":"small talk","title":"","summary":"","decisions":[],"open_questions":[]}`}, nil
	}
	return model.Response{Text: `{"relevant":true,"reason":"a price","title":"Discount agreed",` +
		`"summary":"The team agreed a discount for the renewal.","decisions":["Discount agreed"],"open_questions":[]}`}, nil
}

type pullEnv struct {
	*flushEnv
	history *dispacttest.Server
	lane    *scriptedSummary
	puller  *Puller
	filer   *Filer
	conn    ids.UUID
	now     time.Time
}

func setupPull(t *testing.T) *pullEnv {
	t.Helper()
	e := &pullEnv{flushEnv: setupFlush(t), lane: &scriptedSummary{}, now: linkedAt.Add(6 * time.Hour)}
	e.exec(t, `UPDATE chat_link SET created_at = $2 WHERE id = $1`, e.link, linkedAt)
	if err := e.owner.QueryRow(context.Background(), `SELECT connection_id FROM chat_link WHERE id = $1`, e.link).Scan(&e.conn); err != nil {
		t.Fatal(err)
	}
	e.history = dispacttest.New("team")
	e.history.AddChannel("acme")
	e.history.AddChannel("other")
	srv := httptest.NewServer(e.history)
	t.Cleanup(srv.Close)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	chats := httpChats{client: dispact.New(srv.Client(), srv.URL, "gwapp_test"), connection: e.conn}
	acts := activities.NewStore(db)
	e.puller = &Puller{
		Pool: e.pool, Store: e.store, Chats: chats, Activities: acts, Lane: e.lane,
		Now: func() time.Time { return e.now }, Zone: time.UTC, Log: slog.Default(),
	}
	e.filer = &Filer{Pool: e.pool, Store: e.store, Chats: chats, Activities: acts, Lane: e.lane}
	return e
}

// say adds a message to a channel, minutes after the link was made.
func (e *pullEnv) say(slug string, seq int64, minutes int, text string) {
	e.sayIn(slug, dispacttest.Message{Seq: seq, Text: text}, minutes)
}

// sayIn adds a message shaped by the test, as Rita Rep, minutes after the
// link was made.
func (e *pullEnv) sayIn(slug string, m dispacttest.Message, minutes int) {
	m.UserName, m.PostedAt = "Rita Rep", linkedAt.Add(time.Duration(minutes)*time.Minute)
	e.history.Seed(slug, m)
}

func (e *pullEnv) pull(t *testing.T) {
	t.Helper()
	if err := e.puller.Pull(principal.WithWorkspaceID(context.Background(), e.ws), e.conn); err != nil {
		t.Fatalf("pull: %v", err)
	}
}

func (e *pullEnv) cursor(t *testing.T) *int64 {
	t.Helper()
	var cursor *int64
	if err := e.owner.QueryRow(context.Background(), `SELECT history_cursor FROM chat_link WHERE id = $1`, e.link).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	return cursor
}

type filedNote struct {
	subject, body, sourceID string
	raw                     map[string]any
}

func (e *pullEnv) notes(t *testing.T) []filedNote {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `SELECT a.subject, a.body, a.source_id, a.raw FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.source_system = 'dispact' ORDER BY a.source_id`, e.deal)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (filedNote, error) {
		var n filedNote
		return n, row.Scan(&n.subject, &n.body, &n.sourceID, &n.raw)
	})
	if err != nil {
		t.Fatal(err)
	}
	return notes
}

func TestAPullFilesOneNoteForAClosedEpisodeAndAReplayFilesNone(t *testing.T) {
	e := setupPull(t)
	e.say("acme", 1, 60, "Acme wants 12% off the secret-plan-zebra renewal")
	e.say("acme", 2, 70, "OK, we can give 12% if they sign this week")
	e.say("acme", 3, e.minutesAgo(5), "Still talking about it")
	e.pull(t)

	notes := e.notes(t)
	if len(notes) != 1 || notes[0].subject != "Discount agreed" || notes[0].sourceID != "acme:1" {
		t.Fatalf("notes = %+v, want one note keyed acme:1", notes)
	}
	if !strings.HasPrefix(notes[0].body, "The team agreed a discount for the renewal.") {
		t.Errorf("body = %q, want the summary", notes[0].body)
	}
	if notes[0].raw["message_count"] != float64(2) || !strings.HasSuffix(notes[0].raw["url"].(string), "/channels/acme") {
		t.Errorf("raw = %v, want the episode's count and the link to its channel", notes[0].raw)
	}
	if c := e.cursor(t); c == nil || *c != 2 {
		t.Fatalf("cursor = %v, want 2: the open episode at seq 3 stays after it", c)
	}

	e.pull(t)
	e.exec(t, `UPDATE chat_link SET history_cursor = NULL WHERE id = $1`, e.link)
	e.pull(t)
	if n := len(e.notes(t)); n != 1 {
		t.Fatalf("a second pull and a replay from the start filed %d notes, want 1", n)
	}
	e.assertNoTableHolds(t, "secret-plan-zebra")
}

func TestAnIrrelevantEpisodeFilesNothingAndMovesTheCursor(t *testing.T) {
	e := setupPull(t)
	e.say("acme", 1, 60, "lunch at noon?")
	e.say("acme", 2, 61, "sure")
	e.pull(t)

	if n := len(e.notes(t)); n != 0 {
		t.Fatalf("small talk filed %d notes", n)
	}
	if c := e.cursor(t); c == nil || *c != 2 {
		t.Fatalf("cursor = %v, want 2 past the judged episode", c)
	}
}

func TestWithNoModelAnEpisodeIsPassedOverUnfiled(t *testing.T) {
	e := setupPull(t)
	e.puller.Lane = nil
	e.say("acme", 1, 60, "Acme wants 12% off")
	e.pull(t)

	if n := len(e.notes(t)); n != 0 {
		t.Fatalf("no model filed %d notes", n)
	}
	if c := e.cursor(t); c == nil || *c != 1 {
		t.Fatalf("cursor = %v, want 1: the channel does not wait for a model", c)
	}
}

func TestTalkFromBeforeTheLinkIsJudgedOnlyWhenAskedAndAtMostFiveTimes(t *testing.T) {
	e := setupPull(t)
	for i := range 7 {
		e.say("acme", int64(i+1), -60*(7-i)-30, "price talk "+strconv.Itoa(i))
	}
	e.say("acme", 8, 30, "after the link: lunch?")

	e.pull(t)
	if n := len(e.notes(t)); n != 0 {
		t.Fatalf("a link that did not ask summarised the talk so far: %d notes", n)
	}

	e.exec(t, `UPDATE chat_link SET history_cursor = NULL, backfill_requested = true WHERE id = $1`, e.link)
	e.pull(t)
	notes := e.notes(t)
	if len(notes) != backfillEpisodes || notes[0].sourceID != "acme:3" {
		t.Fatalf("notes = %d starting %v, want the 5 latest episodes from acme:3", len(notes), notes)
	}
	var asked bool
	if err := e.owner.QueryRow(context.Background(), `SELECT backfill_requested FROM chat_link WHERE id = $1`, e.link).Scan(&asked); err != nil || asked {
		t.Fatalf("backfill_requested = %t, %v; want it cleared", asked, err)
	}
	if c := e.cursor(t); c == nil || *c != 7 {
		t.Fatalf("cursor = %v, want 7 where the link began", c)
	}
}

func TestEachLinkReadsAndFilesOnlyItsOwnChannel(t *testing.T) {
	e := setupPull(t)
	project := ids.NewV7()
	e.exec(t, `INSERT INTO project (id, name, source, captured_by) VALUES ($1, 'Other rollout', 'seed', 'system')`, project)
	e.exec(t, `INSERT INTO chat_link (connection_id, provider, channel_slug, channel_name, project_id, linked_by, created_at)
		VALUES ($1, 'dispact', 'other', 'Other', $2, $3, $4)`, e.conn, project, e.rep, linkedAt)
	e.say("other", 1, 60, "The rollout slips a week")
	e.say("acme", 1, 60, "Acme wants 12% off")
	e.pull(t)

	if e.history.Reads("other") == 0 || e.history.Reads("acme") == 0 {
		t.Fatalf("reads = %d of other and %d of acme, want both channels read", e.history.Reads("other"), e.history.Reads("acme"))
	}
	if notes := e.notes(t); len(notes) != 1 || notes[0].sourceID != "acme:1" {
		t.Fatalf("the deal's notes = %+v, want only its own channel's acme:1", notes)
	}
	var onProject string
	if err := e.owner.QueryRow(context.Background(), `SELECT a.source_id FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.project_id = $1`, project).Scan(&onProject); err != nil || onProject != "other:1" {
		t.Fatalf("the project's note = %q, %v; want other:1", onProject, err)
	}
	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "#other:1", false); !errors.Is(err, chatlinks.ErrMessageNotFound) {
		t.Fatalf("filing another channel's message = %v, want ErrMessageNotFound", err)
	}
}

func TestAChosenMessageIsFiledOnceAsASummary(t *testing.T) {
	e := setupPull(t)
	e.say("acme", 1, 60, "The 12% is fine with me")
	note, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "<m:#acme:1|this message>", false)
	if err != nil {
		t.Fatal(err)
	}
	if note.ChatEpisode == nil || note.ChatEpisode.MessageCount != 1 {
		t.Fatalf("chat_episode = %+v, want one message", note.ChatEpisode)
	}
	again, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", false)
	if err != nil || again.Id != note.Id {
		t.Fatalf("filing it again = %v, %v; want the same note", again.Id, err)
	}
	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "9", false); !errors.Is(err, chatlinks.ErrMessageNotFound) {
		t.Fatalf("a missing message = %v, want ErrMessageNotFound", err)
	}
}

func TestAChosenMessageFromBeforeTheLinkAnswersAsMissing(t *testing.T) {
	e := setupPull(t)
	e.say("acme", 1, -30, "Before the link: Acme may walk")
	e.say("acme", 2, 60, "After the link: the 12% is fine")
	for _, thread := range []bool{false, true} {
		if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", thread); !errors.Is(err, chatlinks.ErrMessageNotFound) {
			t.Fatalf("filing a message from before the link (thread %t) = %v, want ErrMessageNotFound", thread, err)
		}
	}
	if n := len(e.notes(t)); n != 0 {
		t.Fatalf("talk from before the link was filed: %d notes", n)
	}
}

func TestAChosenThreadLeavesOutAReplyStampedBeforeTheLink(t *testing.T) {
	e := setupPull(t)
	e.sayIn("acme", dispacttest.Message{Seq: 1, Text: "Can we do 12%?", ReplyCount: 2}, 60)
	e.sayIn("acme", dispacttest.Message{Seq: 2, Text: "Stamped before the link", ThreadSeq: 1}, -30)
	e.sayIn("acme", dispacttest.Message{Seq: 3, Text: "Yes, 12% is fine", ThreadSeq: 1}, 70)
	note, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", true)
	if err != nil {
		t.Fatal(err)
	}
	if note.ChatEpisode == nil || note.ChatEpisode.MessageCount != 2 {
		t.Fatalf("chat_episode = %+v, want the message and the one reply since the link", note.ChatEpisode)
	}
}

func (e *pullEnv) memberCtx() context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, chatLinksPrincipal)
}

// minutesAgo is the minute offset from the link that lies n minutes before now.
func (e *pullEnv) minutesAgo(n int) int {
	return int(e.now.Sub(linkedAt).Minutes()) - n
}

// assertNoTableHolds fails when any row of any table carries text, so the
// chat's words stay in the chat.
func (e *pullEnv) assertNoTableHolds(t *testing.T, text string) {
	t.Helper()
	ctx := context.Background()
	rows, err := e.owner.Query(ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(tables) < 100 {
		t.Fatalf("read %d tables (%v); the scan would pass vacuously", len(tables), err)
	}
	for _, table := range tables {
		var hits int
		query := `SELECT count(*) FROM ` + pgx.Identifier{table}.Sanitize() + ` t WHERE t::text LIKE '%' || $1 || '%'`
		if err := e.owner.QueryRow(ctx, query, text).Scan(&hits); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if hits > 0 {
			t.Errorf("table %s holds the chat's words %q", table, text)
		}
	}
}

// unlinkingSummary is a model call during which a member unlinks the channel.
type unlinkingSummary struct {
	scriptedSummary
	unlink func()
}

func (u *unlinkingSummary) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	u.unlink()
	return u.scriptedSummary.Complete(ctx, req)
}

func TestAnUnlinkWhileTheModelReadsFilesNothing(t *testing.T) {
	e := setupPull(t)
	e.puller.Lane = &unlinkingSummary{unlink: func() {
		e.exec(t, `UPDATE chat_link SET archived_at = now() WHERE id = $1`, e.link)
	}}
	e.say("acme", 1, 60, "Acme wants 12% off")
	e.pull(t)

	if n := len(e.notes(t)); n != 0 {
		t.Fatalf("a channel unlinked during the model call filed %d notes on its old record", n)
	}
}

func TestAnEpisodeCutByThePageCapIsFiledOnceWhole(t *testing.T) {
	e := setupPull(t)
	e.now = linkedAt.Add(12 * time.Hour)
	for seq := int64(1); seq <= 10; seq++ {
		e.say("acme", seq, 60+int(seq), "Acme price talk")
	}
	for seq := int64(11); seq <= 500; seq++ {
		e.say("acme", seq, 120+int(seq), "Acme rollout talk")
	}
	e.pull(t)
	if notes := e.notes(t); len(notes) != 1 || notes[0].sourceID != "acme:1" {
		t.Fatalf("first pull filed %+v, want only the episode the page cap did not cut", notes)
	}

	e.pull(t)
	notes := e.notes(t)
	if len(notes) != 2 || notes[1].sourceID != "acme:11" || notes[1].raw["message_count"] != float64(490) {
		t.Fatalf("second pull filed %+v, want the cut episode once, all 490 messages", notes)
	}
}

func TestAnEpisodeThatFillsTheWholeReadIsFiledSoTheLinkMovesOn(t *testing.T) {
	e := setupPull(t)
	e.now = linkedAt.Add(15 * time.Hour)
	for seq := int64(1); seq <= 600; seq++ {
		e.say("acme", seq, 60+int(seq), "Acme rollout talk")
	}
	e.pull(t)

	notes := e.notes(t)
	if len(notes) != 1 || notes[0].raw["message_count"] != float64(pullPages*dispact.HistoryPageLimit) {
		t.Fatalf("notes = %+v, want the read's one episode filed whole", notes)
	}
	if c := e.cursor(t); c == nil || *c != int64(pullPages*dispact.HistoryPageLimit) {
		t.Fatalf("cursor = %v, want the end of the read", c)
	}
}

func TestAThreadLongerThanOneReadIsNotFiledInPart(t *testing.T) {
	e := setupPull(t)
	e.sayIn("acme", dispacttest.Message{Seq: 1, Text: "Thread talk", ReplyCount: 599}, 60)
	for seq := int64(2); seq <= 600; seq++ {
		e.sayIn("acme", dispacttest.Message{Seq: seq, Text: "Thread talk", ThreadSeq: 1}, 60)
	}
	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", true); !errors.Is(err, chatlinks.ErrThreadTooLong) {
		t.Fatalf("filing a long thread = %v, want ErrThreadTooLong", err)
	}
	if n := len(e.notes(t)); n != 0 {
		t.Fatalf("a part of the thread was filed: %d notes", n)
	}
}

func TestANoteUnderAMessageKeyThatTheChatDidNotWriteIsRefused(t *testing.T) {
	e := setupPull(t)
	e.say("acme", 1, 60, "The 12% is fine with me")
	planted := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, source, source_system, source_id, raw, captured_by)
		VALUES ($1, 'note', 'Fake', 'manual', 'dispact', 'acme:1:message', '{"message_count":1,"url":"https://evil.test"}', 'human:x')`, planted)

	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", false); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("filing over a planted note = %v, want a conflict", err)
	}
}
