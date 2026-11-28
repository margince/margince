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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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
	contactStore := contacts.NewStore(db)
	e.puller = &Puller{
		Pool: e.pool, Store: e.store, Chats: chats, Activities: acts, Contacts: contactStore, Lane: e.lane,
		Now: func() time.Time { return e.now }, Zone: time.UTC, Log: slog.Default(),
	}
	e.filer = &Filer{Pool: e.pool, Store: e.store, Chats: chats, Activities: acts, Contacts: contactStore, Lane: e.lane}
	return e
}

// say adds a message to a channel, minutes after the link was made.
func (e *pullEnv) say(slug string, seq int64, minutes int, text string) {
	e.sayIn(slug, dispacttest.Message{Seq: seq, Text: text}, minutes)
}

// sayIn adds a message shaped by the test, as Rita Rep, minutes after the link
// was made. An address lets the restriction check match the author; a test that
// needs a restricted author sets its own.
func (e *pullEnv) sayIn(slug string, m dispacttest.Message, minutes int) {
	m.UserName, m.PostedAt = "Rita Rep", linkedAt.Add(time.Duration(minutes)*time.Minute)
	if m.UserEmail == "" {
		m.UserEmail = "rita@acme.test"
	}
	e.history.Seed(slug, m)
}

func (e *pullEnv) pull(t *testing.T) {
	t.Helper()
	if err := e.puller.Pull(context.Background(), e.conn); err != nil {
		t.Fatalf("pull: %v", err)
	}
}

// key is the source_id the engine files a talk's note under on the test deal.
func (e *pullEnv) key(talk string) string { return talk + ":deal:" + e.deal.String() }

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
		WHERE a.source_system = 'dispact' ORDER BY a.occurred_at, a.source_id`, e.deal)
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
	if len(notes) != 1 || notes[0].subject != "Discount agreed" || notes[0].sourceID != e.key("acme:1") {
		t.Fatalf("notes = %+v, want one note keyed acme:1 and the deal", notes)
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

func TestAnEpisodeWhoseAuthorRestrictedProcessingIsPassedOverUnfiled(t *testing.T) {
	e := setupPull(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Clara Contact', 'manual', 'system')`, contact)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, 'clara@acme.test', 'manual', 'system')`, contact)
	e.exec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:x', 'subject')`, contact)
	e.sayIn("acme", dispacttest.Message{Seq: 1, Text: "Acme wants 12% off the renewal", UserEmail: "clara@acme.test"}, 60)
	e.pull(t)

	if n := len(e.notes(t)); n != 0 {
		t.Fatalf("a restricted author's talk filed %d notes", n)
	}
	if c := e.cursor(t); c == nil || *c != 1 {
		t.Fatalf("cursor = %v, want 1: the channel moves past the passed-over episode", c)
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
	if len(notes) != backfillEpisodes || notes[0].sourceID != e.key("acme:3") {
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
	e.exec(t, `INSERT INTO project (id, name, source, captured_by) VALUES ($1, 'Other rollout', 'manual', 'system')`, project)
	e.exec(t, `INSERT INTO chat_link (connection_id, provider, channel_slug, channel_name, project_id, linked_by, created_at)
		VALUES ($1, 'dispact', 'other', 'Other', $2, $3, $4)`, e.conn, project, e.rep, linkedAt)
	e.say("other", 1, 60, "The rollout slips a week")
	e.say("acme", 1, 60, "Acme wants 12% off")
	e.pull(t)

	if e.history.Reads("other") == 0 || e.history.Reads("acme") == 0 {
		t.Fatalf("reads = %d of other and %d of acme, want both channels read", e.history.Reads("other"), e.history.Reads("acme"))
	}
	if notes := e.notes(t); len(notes) != 1 || notes[0].sourceID != e.key("acme:1") {
		t.Fatalf("the deal's notes = %+v, want only its own channel's acme:1", notes)
	}
	var onProject string
	if err := e.owner.QueryRow(context.Background(), `SELECT a.source_id FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.project_id = $1`, project).Scan(&onProject); err != nil || onProject != "other:1:project:"+project.String() {
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
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())
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
	if notes := e.notes(t); len(notes) != 1 || notes[0].sourceID != e.key("acme:1") {
		t.Fatalf("first pull filed %+v, want only the episode the page cap did not cut", notes)
	}

	e.pull(t)
	notes := e.notes(t)
	if len(notes) != 2 || notes[1].sourceID != e.key("acme:11") || notes[1].raw["message_count"] != float64(490) {
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
		VALUES ($1, 'note', 'Fake', 'manual', 'dispact', $2, '{"message_count":1,"url":"https://evil.test"}', 'human:x')`, planted, e.key("acme:1:message"))

	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", false); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("filing over a planted note = %v, want a conflict", err)
	}
}

// recordingSummary keeps every request it is sent and answers each episode
// with a note that leaves a question open.
type recordingSummary struct{ prompts []string }

func (s *recordingSummary) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.prompts = append(s.prompts, req.Messages[0].Content)
	return model.Response{Text: `{"relevant":true,"reason":"a price","title":"Two-year discount asked",` +
		`"summary":"Acme asked what discount a two-year deal earns.","decisions":[],"open_questions":["Which discount for two years?"]}`}, nil
}

// partyEnv is a pull whose deal has an owner who writes nothing in the
// channel, a company, and stakeholders of every standing. Every member holds
// the management role, so each one may read the names.
type partyEnv struct {
	*pullEnv
	rec   *recordingSummary
	owner ids.UUID
}

func setupParties(t *testing.T) *partyEnv {
	t.Helper()
	e := &partyEnv{pullEnv: setupPull(t), rec: &recordingSummary{}, owner: ids.NewV7()}
	e.puller.Lane = e.rec
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'owen@chat.test', 'Owen Owner')`, e.owner)
	e.exec(t, `UPDATE deal SET owner_id = $2, amount_minor = 123456700, currency = 'EUR' WHERE id = $1`, e.deal, e.owner)
	reader := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rex@chat.test', 'Rex Reader')`, reader)
	e.seedRoles(t, map[ids.UUID]string{reader: "management", e.owner: "management", e.rep: "management"})
	company := ids.NewV7()
	e.exec(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Acme GmbH', 'seed', 'system')`, company)
	e.exec(t, `UPDATE deal SET company_id = $2 WHERE id = $1`, e.deal, company)
	e.stakeholder(t, e.deal, "Clara Buyer", "workspace", false)
	e.stakeholder(t, e.deal, "Pia Private", "owner", false)
	e.stakeholder(t, e.deal, "Eddie Ended", "workspace", true)
	foreign := ids.NewV7()
	e.exec(t, `INSERT INTO deal (id, name, status, pipeline_id, stage_id, source, captured_by)
		SELECT $2, 'Other deal', 'open', pipeline_id, stage_id, 'manual', 'system' FROM deal WHERE id = $1`, e.deal, foreign)
	e.stakeholder(t, foreign, "Olga Other", "workspace", false)
	return e
}

func (e *partyEnv) stakeholder(t *testing.T, deal ids.UUID, name, visibility string, ended bool) {
	t.Helper()
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'seed', 'system')`, contact, name)
	if visibility == "owner" {
		e.exec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, contact, e.rep)
	}
	e.exec(t, `INSERT INTO relationship (kind, deal_id, contact_id, source, captured_by, ended_at)
		VALUES ('deal_stakeholder', $1, $2, 'seed', 'system', CASE WHEN $3 THEN DATE '2026-01-01' END)`, deal, contact, ended)
}

// secondRequest runs a day of talk and a second one a day later, and returns
// the request the second one sent.
func (e *partyEnv) secondRequest(t *testing.T) string {
	t.Helper()
	e.say("acme", 1, 60, "What discount can we offer for a two-year deal?")
	e.say("acme", 2, 24*60+60, "agreed: option B, 7 percent")
	e.say("acme", 3, e.minutesAgo(5), "still talking")
	e.pull(t)
	if len(e.rec.prompts) != 2 {
		t.Fatalf("model requests = %d, want one per closed episode", len(e.rec.prompts))
	}
	if strings.Contains(e.rec.prompts[0], "Earlier note") {
		t.Errorf("the first episode has no earlier note, got %q", e.rec.prompts[0])
	}
	return e.rec.prompts[1]
}

func requireIn(t *testing.T, prompt string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(prompt, w) {
			t.Errorf("request lacks %q: %q", w, prompt)
		}
	}
}

func requireNotIn(t *testing.T, prompt string, banned ...string) {
	t.Helper()
	for _, b := range banned {
		if strings.Contains(prompt, b) {
			t.Errorf("request carries %q: %q", b, prompt)
		}
	}
}

func TestALaterEpisodeIsSentTheEarlierNoteAndOnlyWhatEveryReaderMayRead(t *testing.T) {
	e := setupParties(t)
	second := e.secondRequest(t)
	requireIn(t, second, "Which discount for two years?", "Acme GmbH", "Clara Buyer", "Owen Owner", "agreed: option B")
	requireNotIn(t, second, "1234567", "Pia Private", "Olga Other", "Eddie Ended")
}

func TestAReaderWithoutTheCompanyOrContactOrEdgeReadLeavesThatNameOut(t *testing.T) {
	for _, tc := range []struct {
		name, object      string
		present, withheld string
	}{
		{"company", "company", "Clara Buyer", "Acme GmbH"},
		{"contact", "contact", "Acme GmbH", "Clara Buyer"},
		{"edge", "relationship", "Acme GmbH", "Clara Buyer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupParties(t)
			e.exec(t, `UPDATE role SET permissions = jsonb_set(permissions, ARRAY['objects', $1, 'read'], 'false') WHERE key = 'management'`, tc.object)
			second := e.secondRequest(t)
			requireIn(t, second, tc.present, "Owen Owner")
			requireNotIn(t, second, tc.withheld)
		})
	}
}

// outsider holds the rep role, which this test strips of deal read, so the
// summary note is not theirs to read.
func (e *partyEnv) outsider(t *testing.T) ids.UUID {
	t.Helper()
	outsider := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'otto@chat.test', 'Otto Outsider')`, outsider)
	e.exec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'rep' AND is_system`, outsider)
	e.denyRead(t, "rep", "deal")
	return outsider
}

func (e *partyEnv) denyRead(t *testing.T, role, object string) {
	t.Helper()
	e.exec(t, `UPDATE role SET permissions = jsonb_set(permissions, ARRAY['objects', $2, 'read'], 'false') WHERE key = $1`, role, object)
}

func TestAMemberWhoCannotReadTheDealDoesNotBoundTheNames(t *testing.T) {
	e := setupParties(t)
	e.outsider(t)
	e.denyRead(t, "rep", "contact")
	second := e.secondRequest(t)
	requireIn(t, second, "Acme GmbH", "Clara Buyer")
}

func TestAMemberWhoCanReadTheDealBoundsTheNames(t *testing.T) {
	e := setupParties(t)
	e.exec(t, `DELETE FROM role_assignment WHERE user_id = $1`, e.owner)
	e.exec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'rep' AND is_system`, e.owner)
	e.denyRead(t, "rep", "contact")
	second := e.secondRequest(t)
	requireIn(t, second, "Acme GmbH")
	requireNotIn(t, second, "Clara Buyer")
}

func (e *partyEnv) chatNotesAs(t *testing.T, user ids.UUID, query *string) int {
	t.Helper()
	var found int
	ws := context.Background()
	err := database.WithTx(ws, e.pool, func(tx pgx.Tx) error {
		members, err := identity.MemberPrincipalsTx(ws, tx, []ids.UUID{user})
		if err != nil {
			return err
		}
		kind := "note"
		rows, _, err := e.puller.Activities.ListActivitiesIn(principal.WithActor(ws, members[user]), tx, activities.ListActivitiesInput{
			Kind: &kind, Query: query,
		})
		for _, row := range rows {
			if row.SourceSystem != nil && *row.SourceSystem == provenance.ChatSummarySource {
				found++
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestTheFiledNoteIsReadByDealReadersOnlyInTheListAndInSearch(t *testing.T) {
	e := setupParties(t)
	outsider := e.outsider(t)
	e.secondRequest(t)
	search := "discount"
	for name, query := range map[string]*string{"list": nil, "search": &search} {
		if got := e.chatNotesAs(t, e.owner, query); got != 2 {
			t.Errorf("%s: a deal reader sees %d chat notes, want both", name, got)
		}
		if got := e.chatNotesAs(t, outsider, query); got != 0 {
			t.Errorf("%s: a member without deal read sees %d chat notes, want none", name, got)
		}
	}
}

func TestADepartedOwnerIsNotNamed(t *testing.T) {
	e := setupParties(t)
	e.exec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.owner)
	second := e.secondRequest(t)
	requireNotIn(t, second, "Owen Owner", "Owner:")
	requireIn(t, second, "Acme GmbH")
}

func TestANoteLimitedToItsParticipantsIsNotSent(t *testing.T) {
	e := setupParties(t)
	e.secondRequest(t)
	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE source_system = 'dispact'`)
	e.exec(t, `DELETE FROM activity WHERE source_id = $1`, e.key("acme:2"))
	e.exec(t, `UPDATE chat_link SET history_cursor = 1 WHERE id = $1`, e.link)
	e.pull(t)
	requireNotIn(t, e.rec.prompts[len(e.rec.prompts)-1], "Which discount for two years?")
}

// note files a summary note as the engine does. Its talk runs from occurred to
// ended, and its key names the first message.
func (e *pullEnv) note(t *testing.T, key, body string, occurred, ended time.Time) {
	t.Helper()
	id := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source_system, source_id, raw, audience, source, captured_by)
		VALUES ($1, 'note', $2, $2, $3, 'dispact', $4, jsonb_build_object('ended_at', $5::text), 'workspace', 'chat_links', 'system')`,
		id, body, occurred, key, ended.UTC().Format(time.RFC3339))
	e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, e.deal)
}

func (e *pullEnv) previous(t *testing.T, before time.Time) string {
	t.Helper()
	link := chatlinks.Link{DealID: &e.deal, ChannelSlug: "acme"}
	var got string
	err := database.WithTx(context.Background(), e.pool, func(tx pgx.Tx) error {
		var err error
		got, err = previousNote(context.Background(), tx, link, before)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestThePreviousNoteIsChosenByWhenItsTalkEndedThenByChannelSequence(t *testing.T) {
	e := setupPull(t)
	at := func(minutes int) time.Time { return linkedAt.Add(time.Duration(minutes) * time.Minute) }
	// A thread chosen later, rooted before the episode, whose replies run past it.
	e.note(t, "acme:5:thread", "overlapping thread", at(10), at(300))
	// Two notes whose talk ended together: the higher sequence is the later one.
	e.note(t, "acme:3", "ended together, seq 3", at(60), at(90))
	e.note(t, "acme:10", "ended together, seq 10", at(20), at(90))
	// Filed in the reverse order of the talk, with an earlier end.
	e.note(t, "acme:1", "ended before", at(80), at(85))

	if got := e.previous(t, at(120)); !strings.HasPrefix(got, "ended together, seq 10") {
		t.Errorf("previous note = %q, want the one that ended last, higher sequence first", got)
	}
	if got := e.previous(t, at(88)); !strings.HasPrefix(got, "ended before") {
		t.Errorf("previous note = %q, want the note that ended strictly before the episode", got)
	}
	if got := e.previous(t, at(90)); !strings.HasPrefix(got, "ended before") {
		t.Errorf("previous note = %q, want a note ending exactly at the start left out", got)
	}
	if got := e.previous(t, at(5)); got != "" {
		t.Errorf("previous note = %q, want none", got)
	}
}
