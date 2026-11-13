// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fakeChat is the chat as the bot sees it: channels by slug, and the emails
// each one was opened to.
type fakeChat struct {
	mu       sync.Mutex
	channels map[string]dispact.Channel
	members  map[string][]string
}

func (f *fakeChat) Me(context.Context) (dispact.Bot, error) { return dispact.Bot{TeamID: "team"}, nil }

func (f *fakeChat) Channel(_ context.Context, slug string) (dispact.Channel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch, ok := f.channels[slug]
	if !ok {
		return dispact.Channel{}, dispact.ErrChannelGone
	}
	return ch, nil
}

func (f *fakeChat) CreateChannel(_ context.Context, key, name string) (dispact.Channel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := dispact.Channel{Slug: "c-" + key[len(key)-8:], Name: name, IsPrivate: true, BotIsMember: true}
	f.channels[ch.Slug] = ch
	return ch, nil
}

func (f *fakeChat) AddMembers(_ context.Context, slug string, emails []string) (dispact.MemberResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[slug] = append(f.members[slug], emails...)
	return dispact.MemberResult{Added: emails}, nil
}

func (f *fakeChat) Post(context.Context, string, string, string, string) (dispact.Posted, error) {
	return dispact.Posted{ID: "m"}, nil
}

func (f *fakeChat) Update(context.Context, string, string) error { return nil }

func (f *fakeChat) History(context.Context, string, int64, int) (dispact.HistoryPage, error) {
	return dispact.HistoryPage{}, nil
}

type fakeChats struct {
	chat       *fakeChat
	connection ids.UUID
}

func (f fakeChats) Open(context.Context) (Session, error) {
	return Session{ConnectionID: f.connection, Address: "https://chat.test", Chat: f.chat}, nil
}

// flushes counts the flushes scheduled per link.
type flushes struct {
	mu    sync.Mutex
	links map[ids.UUID]int
}

func (f *flushes) EnqueueFlushTx(_ context.Context, _ pgx.Tx, link ids.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[link]++
	return nil
}

type linkEnv struct {
	owner                 *pgx.Conn
	store                 *Store
	chat                  *fakeChat
	flush                 *flushes
	ws, rep, other        ids.UUID
	deal, second, project ids.UUID
}

func setupLinks(t *testing.T) *linkEnv {
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
	e := &linkEnv{owner: owner, ws: ids.NewV7(), rep: ids.NewV7(), other: ids.NewV7(),
		deal: ids.NewV7(), second: ids.NewV7(), project: ids.NewV7()}
	e.exec(t, `INSERT INTO workspace (id) VALUES ($1)`, e.ws)
	for _, u := range []ids.UUID{e.rep, e.other} {
		e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`, u, "rep-"+u.String()+"@chat.test")
	}
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Chat')`, pipeline)
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	company := ids.NewV7()
	e.exec(t, `INSERT INTO company (id, display_name, owner_id, source, captured_by) VALUES ($1, 'Acme', $2, 'seed', 'system')`, company, e.rep)
	e.exec(t, `INSERT INTO project (id, name, owner_id, source, captured_by) VALUES ($1, 'Rollout', $2, 'seed', 'system')`, e.project, e.rep)
	e.exec(t, `INSERT INTO relationship (kind, project_id, company_id, role, source, captured_by)
		VALUES ('project_company', $1, $2, 'customer', 'manual', 'human:x')`, e.project, company)
	for _, d := range []struct {
		id      ids.UUID
		project *ids.UUID
	}{{e.deal, &e.project}, {e.second, nil}} {
		e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, project_id, company_id, source, captured_by)
			VALUES ($1, 'Acme renewal', 'open', $2, $3, $4, $5, $6, 'seed', 'system')`, d.id, e.rep, pipeline, stage, d.project, company)
	}
	connection := ids.NewV7()
	e.exec(t, `INSERT INTO channel_connection (id, provider, channel_id, channel_label, address, credential_ref, status, connected_by)
		VALUES ($1, 'dispact', 'team', 'Margince', 'https://chat.test', 'ref', 'connected', $2)`, connection, e.rep)
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	e.chat = &fakeChat{channels: map[string]dispact.Channel{
		"team-acme": {Slug: "team-acme", Name: "Acme team", IsPrivate: true, BotIsMember: true},
		"lobby":     {Slug: "lobby", Name: "Lobby", IsPrivate: false, BotIsMember: true},
		"secret":    {Slug: "secret", Name: "Secret", IsPrivate: true, BotIsMember: false},
	}, members: map[string][]string{}}
	e.flush = &flushes{links: map[ids.UUID]int{}}
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)), fakeChats{chat: e.chat, connection: connection}, e.flush)
	return e
}

func (e *linkEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *linkEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *linkEnv) seat(user ids.UUID, scope principal.RowScope) context.Context {
	grant := principal.ObjectGrant{Read: true, Update: true}
	return e.seatWith(user, scope, map[string]principal.ObjectGrant{"deal": grant, "project": grant})
}

func (e *linkEnv) seatWith(user ids.UUID, scope principal.RowScope, grants map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RoleKeys: []string{"rep"}, RowScope: scope, Objects: grants},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func (e *linkEnv) system() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:chat_links",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
}

func TestCreatingAChannelLinksTheDealWithItsProjectAndInvitesTheRep(t *testing.T) {
	e := setupLinks(t)
	state, err := e.store.Link(e.seat(e.rep, principal.RowScopeOwn), RecordDeal, e.deal, LinkRequest{Mode: ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if state.State != StateLinked || state.Link.ProjectID == nil || *state.Link.ProjectID != e.project {
		t.Fatalf("state = %+v, want linked with the deal's project", state)
	}
	if state.Invites == nil || state.Invites.Invited != 1 || len(e.chat.members[state.Link.ChannelSlug]) != 1 {
		t.Fatalf("invites = %+v to %v, want the rep once (the linker is also the owner)", state.Invites, e.chat.members)
	}
	project, err := e.store.GetState(e.seat(e.rep, principal.RowScopeOwn), RecordProject, e.project)
	if err != nil || project.Link == nil || project.Link.ID != state.Link.ID {
		t.Fatalf("project state = %+v, %v; want the deal's channel", project, err)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'chat_link' AND entity_id = $1`, state.Link.ID); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
	if e.flush.links[state.Link.ID] != 1 {
		t.Errorf("flushes = %v, want one to post the card", e.flush.links)
	}
}

func TestAnExistingChannelMustExistBePrivateAndHoldTheBot(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep, principal.RowScopeOwn)
	no := false
	for channel, want := range map[string]error{"lobby": ErrChannelNotPrivate, "secret": ErrBotNotMember, "nowhere": ErrChannelNotFound} {
		_, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeExisting, Channel: channel, SummarizeHistory: &no})
		if !errors.Is(err, want) {
			t.Errorf("linking %s = %v, want %v", channel, err, want)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM chat_link`); n != 0 {
		t.Fatalf("refused links wrote %d rows", n)
	}
}

func TestARecordAndAChannelCarryOneLinkEach(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep, principal.RowScopeOwn)
	yes := true
	existing := LinkRequest{Mode: ModeExisting, Channel: "https://chat.test/channels/team-acme", SummarizeHistory: &yes}
	first, err := e.store.Link(ctx, RecordDeal, e.deal, existing)
	if err != nil || !first.Link.BackfillRequested {
		t.Fatalf("first link = %+v, %v; want linked with the history asked for", first, err)
	}
	if _, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeCreate}); !errors.Is(err, ErrRecordAlreadyLinked) {
		t.Errorf("a second channel for the deal = %v, want record_already_linked", err)
	}
	if _, err := e.store.Link(ctx, RecordDeal, e.second, existing); !errors.Is(err, ErrChannelAlreadyLinked) {
		t.Errorf("the channel for a second deal = %v, want channel_already_linked", err)
	}
}

func TestARepWhoMayNotEditTheDealCannotLinkIt(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.other, principal.RowScopeOwn)
	if _, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeCreate}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("link by a rep who does not own the deal = %v, want permission denied", err)
	}
	if len(e.chat.channels) != 3 {
		t.Errorf("a refused link still made a channel: %v", e.chat.channels)
	}
	if state, err := e.store.GetState(ctx, RecordDeal, e.deal); err != nil || state.State != StateNone {
		t.Errorf("a reader of the deal sees %+v, %v; want none", state, err)
	}
}

func TestAReplayedEventAddsNothingAndASilentLinkRecordsNothing(t *testing.T) {
	e := setupLinks(t)
	state, err := e.store.Link(e.seat(e.rep, principal.RowScopeOwn), RecordDeal, e.deal, LinkRequest{Mode: ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	ev := Event{EventID: ids.NewV7(), EventType: "deal.stage_changed", EntityType: "deal", EntityID: e.deal,
		Tier: TierMilestone, Detail: map[string]any{"to_stage_name": "Won"}, Deals: []ids.UUID{e.deal}}
	for range 2 {
		if err := e.store.RecordEvent(e.system(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM chat_post_item WHERE link_id = $1`, state.Link.ID); n != 1 {
		t.Fatalf("a replayed event left %d items, want 1", n)
	}
	if _, err := e.store.SetVerbosity(e.seat(e.rep, principal.RowScopeOwn), RecordDeal, e.deal, VerbositySilent); err != nil {
		t.Fatal(err)
	}
	ev.EventID = ids.NewV7()
	if err := e.store.RecordEvent(e.system(), ev); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM chat_post_item WHERE link_id = $1`, state.Link.ID); n != 1 {
		t.Fatalf("a silent link recorded an item: %d", n)
	}
	if e.flush.links[state.Link.ID] < 3 {
		t.Errorf("flushes = %d, want one per event so the card stays current", e.flush.links[state.Link.ID])
	}
}

func TestANarrowedActivityIsWithdrawnAndItsPostRewritten(t *testing.T) {
	e := setupLinks(t)
	state, err := e.store.Link(e.seat(e.rep, principal.RowScopeOwn), RecordDeal, e.deal, LinkRequest{Mode: ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	activity := ids.NewV7()
	for _, ev := range []Event{
		{EventID: ids.NewV7(), EventType: "activity.captured", EntityType: "activity", EntityID: activity, ActivityID: &activity, Tier: TierImportant, Deals: []ids.UUID{e.deal}},
		{EventID: ids.NewV7(), EventType: "offer.sent", EntityType: "offer", EntityID: ids.NewV7(), Tier: TierMilestone, Deals: []ids.UUID{e.deal}},
	} {
		if err := e.store.RecordEvent(e.system(), ev); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := e.store.PendingFor(e.system(), state.Link.ID)
	if err != nil || len(pending.Items) != 2 {
		t.Fatalf("pending = %+v, %v; want two items", pending, err)
	}
	carried := []ids.UUID{pending.Items[0].ID, pending.Items[1].ID}
	if err := e.store.MarkPosted(e.system(), state.Link.ID, carried[0], "m1", carried); err != nil {
		t.Fatal(err)
	}
	if err := e.store.Withdraw(e.system(), activity); err != nil {
		t.Fatal(err)
	}
	pending, err = e.store.PendingFor(e.system(), state.Link.ID)
	if err != nil || len(pending.Items) != 0 || len(pending.Rewrites) != 1 || len(pending.Rewrites[0].Items) != 1 {
		t.Fatalf("after withdrawal = %+v, %v; want message m1 rewritten with the offer alone", pending, err)
	}
	if err := e.store.MarkRewritten(e.system(), state.Link.ID, "m1"); err != nil {
		t.Fatal(err)
	}
	if pending, _ = e.store.PendingFor(e.system(), state.Link.ID); len(pending.Rewrites) != 0 {
		t.Fatalf("a rewrite was due twice: %+v", pending.Rewrites)
	}
}

func TestUnlinkingFreesTheChannelForTheRecordAgain(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep, principal.RowScopeOwn)
	no := false
	existing := LinkRequest{Mode: ModeExisting, Channel: "team-acme", SummarizeHistory: &no}
	if _, err := e.store.Link(ctx, RecordDeal, e.second, existing); err != nil {
		t.Fatal(err)
	}
	if err := e.store.Unlink(ctx, RecordDeal, e.second); err != nil {
		t.Fatal(err)
	}
	if state, err := e.store.GetState(ctx, RecordDeal, e.second); err != nil || state.State != StateNone {
		t.Fatalf("after unlink = %+v, %v; want none", state, err)
	}
	if _, err := e.store.Link(ctx, RecordDeal, e.second, existing); err != nil {
		t.Fatalf("linking the channel again = %v", err)
	}
	if err := e.store.Unlink(ctx, RecordDeal, e.deal); !errors.Is(err, ErrLinkNotFound) {
		t.Fatalf("unlinking a record with no channel = %v, want channel_link_not_found", err)
	}
}

func TestADealsProjectJoinsItsChannelOnlyWhenTheRepMayEditTheProject(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seatWith(e.rep, principal.RowScopeOwn, map[string]principal.ObjectGrant{
		"deal": {Read: true, Update: true}, "project": {Read: true}})
	state, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if state.Link.ProjectID != nil {
		t.Fatalf("the project joined a channel its reader may not edit: %+v", state.Link)
	}
}
