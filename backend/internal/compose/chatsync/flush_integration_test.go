// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact/dispacttest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// postTexts are the texts the bot posted, oldest first.
func postTexts(chat *dispacttest.Server) []string {
	posts := chat.Posts()
	out := make([]string, 0, len(posts))
	for _, p := range posts {
		out = append(out, p.Text)
	}
	return out
}

type httpChats struct {
	client     *dispact.Client
	connection ids.UUID
}

func (c httpChats) Open(context.Context) (chatlinks.Session, error) {
	return chatlinks.Session{ConnectionID: c.connection, Address: "https://chat.test", Chat: c.client}, nil
}

type noFlush struct{}

func (noFlush) EnqueueFlushTx(context.Context, pgx.Tx, ids.UUID) error { return nil }

type flushEnv struct {
	owner           *pgx.Conn
	pool            *pgxpool.Pool
	chat            *dispacttest.Server
	store           *chatlinks.Store
	sync            *Sync
	flusher         *Flusher
	ws, rep, deal   ids.UUID
	shared, private ids.UUID
	link            ids.UUID
}

func setupFlush(t *testing.T) *flushEnv {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &flushEnv{owner: owner, ws: ids.NewV7(), rep: ids.NewV7(), deal: ids.NewV7(), shared: ids.NewV7(), private: ids.NewV7()}
	e.exec(t, `INSERT INTO workspace (id) VALUES ($1)`, e.ws)
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rep@chat.test', 'Rita Rep')`, e.rep)
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Chat')`, pipeline)
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Acme renewal', 'open', $2, $3, $4, 'seed', 'system')`, e.deal, e.rep, pipeline, stage)
	for id, row := range map[ids.UUID][2]string{e.shared: {"Shared plan", "workspace"}, e.private: {"Private aside", "participants"}} {
		e.exec(t, `INSERT INTO activity (id, kind, subject, audience, source, captured_by) VALUES ($1, 'note', $2, $3, 'manual', 'human:x')`, id, row[0], row[1])
		e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, e.deal)
	}
	connection := ids.NewV7()
	e.exec(t, `INSERT INTO channel_connection (id, provider, channel_id, channel_label, address, credential_ref, status, connected_by)
		VALUES ($1, 'dispact', 'team', 'Margince', 'https://chat.test', 'ref', 'connected', $2)`, connection, e.rep)
	e.link = ids.NewV7()
	e.exec(t, `INSERT INTO chat_link (id, connection_id, provider, channel_slug, channel_name, deal_id, verbosity, linked_by)
		VALUES ($1, $2, 'dispact', 'acme', 'Acme', $3, 'important', $4)`, e.link, connection, e.deal, e.rep)
	if e.pool, err = testdb.Pool(ctx, appDSN); err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	e.chat = dispacttest.New("team")
	e.chat.AddChannel("acme")
	srv := httptest.NewServer(e.chat)
	t.Cleanup(srv.Close)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	chats := httpChats{client: dispact.New(srv.Client(), srv.URL, "gwapp_test"), connection: connection}
	e.store = chatlinks.NewStore(db, chats, noFlush{}, Viewers)
	e.sync = New(e.pool, db, e.store)
	e.flusher = NewFlusher(e.pool, chats, e.store, "https://crm.test", slog.Default())
	return e
}

func (e *flushEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *flushEnv) handle(t *testing.T, eventType, entity string, id ids.UUID, payload string) {
	t.Helper()
	env := events.Envelope{EventID: ids.NewV7(), Type: eventType, Entity: events.EntityRef{Type: entity, ID: id}, Payload: json.RawMessage(payload)}
	if err := e.sync.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling %s: %v", eventType, err)
	}
}

func (e *flushEnv) flush(t *testing.T) {
	t.Helper()
	if err := e.flusher.Flush(principal.WithWorkspaceID(context.Background(), e.ws), e.link); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

func TestAFlushPostsTheCardAndOnePostWithOnlyWhatTheWorkspaceMayRead(t *testing.T) {
	e := setupFlush(t)
	e.handle(t, "activity.captured", "activity", e.shared, `{"kind":"note"}`)
	e.handle(t, "activity.captured", "activity", e.private, `{"kind":"note"}`)
	e.handle(t, "deal.stage_changed", "deal", e.deal, `{"to_stage_name":"Proposal"}`)
	e.flush(t)

	texts := postTexts(e.chat)
	if len(texts) != 2 {
		t.Fatalf("posts = %q, want the card and one update", texts)
	}
	card, update := texts[0], texts[1]
	if !strings.Contains(card, "*Acme renewal*") || !strings.Contains(card, "<@rep@chat.test|Rita Rep>") ||
		!strings.Contains(card, "https://crm.test/#/deals/"+e.deal.String()) {
		t.Errorf("card = %q, want the deal, its owner and the link back", card)
	}
	if !strings.Contains(update, "Stage moved to Proposal") || !strings.Contains(update, "New note: Shared plan") {
		t.Errorf("update = %q, want the stage and the shared note", update)
	}
	if strings.Contains(update, "Private aside") || strings.Contains(card, "Private aside") {
		t.Fatalf("a private note reached the channel: %q", texts)
	}

	e.flush(t)
	if n := len(postTexts(e.chat)); n != 2 {
		t.Fatalf("a flush with nothing new posted again: %d posts", n)
	}
}

func TestANoteNarrowedAfterPostingIsRewrittenOutOfItsPost(t *testing.T) {
	e := setupFlush(t)
	e.handle(t, "activity.captured", "activity", e.shared, `{"kind":"note"}`)
	e.handle(t, "deal.stage_changed", "deal", e.deal, `{"to_stage_name":"Proposal"}`)
	e.flush(t)

	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, e.shared)
	e.handle(t, "activity.updated", "activity", e.shared, `{"changed_fields":{"audience":"participants"}}`)
	e.flush(t)

	rewritten, ok := e.chat.Updated(dispacttest.TS(2))
	if !ok {
		t.Fatal("the second post was not rewritten")
	}
	if strings.Contains(rewritten, "Shared plan") || !strings.Contains(rewritten, "Stage moved to Proposal") {
		t.Fatalf("rewritten post = %q, want the stage alone", rewritten)
	}
	if n := len(postTexts(e.chat)); n != 2 {
		t.Fatalf("the withdrawal posted anew: %d posts", n)
	}
}

func TestAnArchivedNoteIsRewrittenOutOfItsPost(t *testing.T) {
	e := setupFlush(t)
	e.handle(t, "activity.captured", "activity", e.shared, `{"kind":"note"}`)
	e.handle(t, "deal.stage_changed", "deal", e.deal, `{"to_stage_name":"Proposal"}`)
	e.flush(t)

	e.exec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, e.shared)
	e.handle(t, "activity.archived", "activity", e.shared, `{}`)
	e.flush(t)

	if rewritten, _ := e.chat.Updated(dispacttest.TS(2)); strings.Contains(rewritten, "Shared plan") || rewritten == "" {
		t.Fatalf("rewritten post = %q, want the archived note gone", rewritten)
	}
}

// A contact's erasure restricts their correspondence, and the event rides the
// contact stream naming each activity. A note posted before is rewritten out.
func TestANoteOfARestrictedContactIsRewrittenOutOfItsPost(t *testing.T) {
	e := setupFlush(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Clara Contact', 'manual', 'system')`, contact)
	e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, e.shared, contact)
	e.handle(t, "activity.captured", "activity", e.shared, `{"kind":"note"}`)
	e.handle(t, "deal.stage_changed", "deal", e.deal, `{"to_stage_name":"Proposal"}`)
	e.flush(t)

	e.exec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Privacy desk', 'erasure request')`, e.shared)
	e.exec(t, `UPDATE activity SET restricted_at = now(), archived_at = now(), restricted_reason = 'erasure request',
		restricted_until = now() + interval '1 year', retention_class = 'commercial_correspondence', retention_class_at = now()
		WHERE id = $1`, e.shared)
	restricted := crmcontracts.PublicEventRetentionRestricted{}
	if !groupReads(t, "cg:chat-links", restricted.EventType()) {
		t.Fatal("cg:chat-links does not read the stream retention.restricted rides")
	}
	e.handle(t, restricted.EventType(), restricted.EntityType(), e.shared, `{"action":"restrict"}`)
	e.flush(t)

	rewritten, ok := e.chat.Updated(dispacttest.TS(2))
	if !ok || strings.Contains(rewritten, "Shared plan") || !strings.Contains(rewritten, "Stage moved to Proposal") {
		t.Fatalf("rewritten post = %q (%v), want the stage alone", rewritten, ok)
	}
}

func TestALinkOfAReplacedBotPausesInsteadOfPostingOnTheNewOne(t *testing.T) {
	e := setupFlush(t)
	e.exec(t, `UPDATE channel_connection SET status = 'disconnected', archived_at = now()`)
	e.exec(t, `INSERT INTO channel_connection (provider, channel_id, channel_label, address, credential_ref, status, connected_by)
		VALUES ('dispact', 'other-team', 'Other', 'https://other.test', 'ref2', 'connected', $1)`, e.rep)
	e.flusher.Chats = httpChats{client: e.flusher.Chats.(httpChats).client, connection: ids.NewV7()}
	e.handle(t, "deal.stage_changed", "deal", e.deal, `{"to_stage_name":"Proposal"}`)
	e.flush(t)

	if n := len(postTexts(e.chat)); n != 0 {
		t.Fatalf("a link of a replaced bot posted %d messages", n)
	}
	var status string
	if err := e.owner.QueryRow(context.Background(), `SELECT status FROM chat_link WHERE id = $1`, e.link).Scan(&status); err != nil || status != chatlinks.StatusChannelGone {
		t.Fatalf("link status = %q, %v; want channel_gone", status, err)
	}
}

func groupReads(t *testing.T, name, eventType string) bool {
	t.Helper()
	stream, err := events.StreamFor(eventType)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range events.Groups() {
		if g.Name == name {
			return slices.Contains(g.Streams, stream)
		}
	}
	return false
}
