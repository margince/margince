// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type routedSlackHTTP struct {
	client *http.Client
	base   *url.URL
}

func (r routedSlackHTTP) Do(request *http.Request) (*http.Response, error) {
	forwarded := request.Clone(request.Context())
	target := *request.URL
	target.Scheme, target.Host = r.base.Scheme, r.base.Host
	target.Path = strings.TrimPrefix(target.Path, "/api")
	forwarded.URL = &target
	return r.client.Do(forwarded)
}

type nativeRootSessions struct{ session chatlinks.Session }

func (n nativeRootSessions) Open(context.Context) (chatlinks.Session, error) {
	return chatlinks.Session{}, chatlinks.ErrBotNotConnected
}

func (n nativeRootSessions) OpenProvider(_ context.Context, provider string) (chatlinks.Session, error) {
	if provider != "slack" {
		return chatlinks.Session{}, chatlinks.ErrBotNotConnected
	}
	return n.session, nil
}

type nativeSummaryBoundary struct {
	irrelevant bool
	summary    string
	requests   []model.Request
}

func (n *nativeSummaryBoundary) Complete(_ context.Context, request model.Request) (model.Response, error) {
	n.requests = append(n.requests, request)
	encoded, err := json.Marshal(map[string]any{"relevant": !n.irrelevant, "reason": "pilot approval", "title": "Pilot approval", "summary": n.summary, "decisions": []string{}, "open_questions": []string{}})
	return model.Response{Text: string(encoded)}, err
}

func TestNativeRootCorrectionsReviseOneRecapAndKeepHumanEdits(t *testing.T) {
	e := setupFlush(t)
	if err := e.store.Unlink(e.member(), "deal", e.deal); err != nil {
		t.Fatal(err)
	}
	root := strconv.FormatInt(linkedAt.Add(time.Hour).Unix(), 10) + ".000001"
	source := "The pilot can start only after Legal approves the amended agreement."
	fixture := &nativeRootSource{root: root, source: source, incomplete: true}
	server := serveNativeRootFixture(t, fixture)
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	now := linkedAt.Add(48 * time.Hour)
	bot := pacedSlackFixture(t, routedSlackHTTP{server.Client(), base}, "synthetic-bot", "T_NATIVE", "U_BOT", &now)
	connection := ids.NewV7()
	e.exec(t, `UPDATE channel_connection SET archived_at=now(),status='disconnected'`)
	e.exec(t, `INSERT INTO channel_connection(id,provider,channel_id,channel_label,credential_ref,status,connected_by)
 VALUES($1,'slack','T_NATIVE','Native fixture','sealed-fixture','connected',$2)`, connection, e.rep)
	sessions := nativeRootSessions{chatlinks.Session{Provider: "slack", ConnectionID: connection, TeamID: "T_NATIVE", Chat: bot}}
	store := chatlinks.NewStore(e.db(), sessions, noFlush{}, Viewers)
	memberCtx := connectNativeRecapMember(t, e, store, routedSlackHTTP{server.Client(), base})

	yes := true
	state, err := store.Link(memberCtx, "deal", e.deal, chatlinks.LinkRequest{Provider: "slack", Mode: "existing", Channel: "C_NATIVE", SummarizeHistory: &yes})
	if err != nil {
		t.Fatal(err)
	}
	e.link = state.Link.ID
	lane := &nativeSummaryBoundary{summary: "The pilot remains conditional on Legal approval of the amended agreement."}
	puller := &Puller{Pool: e.pool, Store: store, Chats: sessions, Activities: activities.NewStore(e.db()), Contacts: contacts.NewStore(e.db()), Lane: lane, Now: func() time.Time { return now }, Zone: time.UTC, Log: slog.Default()}
	ctx := principal.WithCorrelationID(principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), chatLinksPrincipal), ids.NewV7())
	pull := func() {
		t.Helper()
		if err := puller.Pull(ctx, connection); err != nil {
			t.Fatal(err)
		}
	}
	read := func() (ids.UUID, string) { return readNativeRecap(t, e) }
	pull()
	var partial int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE source_system=$1 AND source=$2`, provenance.ChatSummarySource, provenance.RecordSourceAutomation).Scan(&partial); err != nil || partial != 0 {
		t.Fatalf("filed a partial native thread: count=%d error=%v", partial, err)
	}
	var applied *string
	if err := e.owner.QueryRow(context.Background(), `SELECT applied_revision FROM chat_root WHERE link_id=$1`, e.link).Scan(&applied); err != nil || applied != nil {
		t.Fatalf("marked an incomplete source as applied: %v %v", applied, err)
	}
	puller = resumeNativeAfterOutage(ctx, t, e, connection, puller, fixture, &now)
	pull()
	original, body := read()
	if !strings.Contains(body, "conditional") || len(lane.requests) != 1 || !strings.Contains(lane.requests[0].Messages[0].Content, source) {
		t.Fatalf("source was not summarized completely: %q requests=%d", body, len(lane.requests))
	}

	humanRead, activityID, binding := assertNativeSourceAccess(memberCtx, t, puller, store, original, activities.ChatSourceBinding{Provider: "slack", Connection: connection, Channel: "C_NATIVE", Root: root})
	fixture.source += " Thanks for checking."
	lane.irrelevant = true
	if err := store.InvalidateNativeRoot(ctx, connection, "C_NATIVE", chatwire.RootRef{ID: root, Revision: "chatter"}); err != nil {
		t.Fatal(err)
	}
	pull()
	same, kept := read()
	if same != original || kept != body {
		t.Fatalf("chatter removed a decision recap: %s %q", same, kept)
	}
	var needsReview bool
	if err := e.owner.QueryRow(context.Background(), `SELECT coalesce((raw->>'source_changed')::boolean,false) FROM activity WHERE id=$1`, original).Scan(&needsReview); err != nil || !needsReview {
		t.Fatalf("changed relevance was not flagged: %v %v", needsReview, err)
	}
	fixture.source = source
	lane.irrelevant = false
	fixture.revised = true
	lane.summary = "Legal approved the amended agreement and the pilot can now start."
	if err := store.InvalidateNativeRoot(ctx, connection, "C_NATIVE", chatwire.RootRef{ID: root, Revision: "edit-one", Deleted: false}); err != nil {
		t.Fatal(err)
	}
	pull()
	updated, body := read()
	if updated != original || !strings.Contains(body, "can now start") {
		t.Fatalf("correction did not revise the same note: %s %q", updated, body)
	}
	human := humanRead
	edited := "Keep my reviewed outcome and next action."
	if _, err := puller.Activities.UpdateActivity(human, ids.ActivityID{UUID: original}, activities.UpdateActivityInput{Body: &edited}); err != nil {
		t.Fatal(err)
	}
	fixture.revised = false
	if err := store.InvalidateNativeRoot(ctx, connection, "C_NATIVE", chatwire.RootRef{ID: root, Revision: "edit-two", Deleted: false}); err != nil {
		t.Fatal(err)
	}
	pull()
	_, body = read()
	if body != edited {
		t.Fatalf("replaced a human edit: %q", body)
	}
	archiveActor, _ := principal.Actor(human)
	archiveActor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true, Delete: true}
	archiveCtx := principal.WithActor(human, archiveActor)
	if _, err := puller.Activities.ArchiveActivity(archiveCtx, activityID, nil); err != nil {
		t.Fatal(err)
	}
	fixture.revised = true
	if err := store.InvalidateNativeRoot(ctx, connection, "C_NATIVE", chatwire.RootRef{ID: root, Revision: "archived-edit"}); err != nil {
		t.Fatal(err)
	}
	pull()
	archived, err := puller.Activities.GetActivity(archiveCtx, activityID, storekit.IncludeArchived)
	if err != nil || archived.ArchivedAt == nil || archived.Body == nil || *archived.Body != edited {
		t.Fatalf("source correction restored or rewrote archived recap: %#v %v", archived, err)
	}
	fixture.gone = true
	if err := store.InvalidateNativeRoot(ctx, connection, "C_NATIVE", chatwire.RootRef{ID: root, Revision: "unavailable"}); err != nil {
		t.Fatal(err)
	}
	pull()
	_, body = read()
	if body != edited {
		t.Fatalf("unavailable source was mistaken for deletion: %q", body)
	}
	if err := store.InvalidateNativeRoot(ctx, connection, "C_NATIVE", chatwire.RootRef{ID: root, Revision: "delete-root", Deleted: true}); err != nil {
		t.Fatal(err)
	}
	pull()
	_, body = read()
	archived, err = puller.Activities.GetActivity(archiveCtx, activityID, storekit.IncludeArchived)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("source deletion restored archived recap: %#v %v", archived, err)
	}
	if strings.Contains(body, "reviewed outcome") || strings.Contains(body, "agreement") || !strings.Contains(body, "no longer available") {
		t.Fatalf("deleted source retained its derived content: %q", body)
	}
	if _, err := puller.Activities.ChatSource(humanRead, activityID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted source still available: %v", err)
	}
	if _, err := store.VerifySource(humanRead, binding.Provider, binding.Connection, binding.Channel, binding.Root); !errors.Is(err, messaging.ErrUnavailable) {
		t.Fatalf("missing source proof: %v", err)
	}
	if err := store.DisconnectMember(humanRead, "slack"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.VerifySource(humanRead, binding.Provider, binding.Connection, binding.Channel, binding.Root); err == nil {
		t.Fatal("revoked grant retained source access")
	}
	if err := store.InvalidateNativeRoot(ctx, connection, "C_NATIVE", chatwire.RootRef{ID: "late-old-root", Revision: "new-reply"}); err != nil {
		t.Fatal(err)
	}
	var noticeRevision *string
	if err := e.owner.QueryRow(context.Background(), `SELECT notice_revision FROM chat_root WHERE link_id=$1 AND root_id='late-old-root'`, e.link).Scan(&noticeRevision); err != nil || noticeRevision == nil || *noticeRevision != "new-reply" {
		t.Fatalf("notice lost a previously unscanned root: %v %v", noticeRevision, err)
	}
	assertNativeRecapAuditBoundary(t, e, original, connection, root)
}

type nativeRootSource struct {
	root, source                       string
	incomplete, revised, gone, limited bool
}

func serveNativeRootFixture(t *testing.T, fixture *nativeRootSource) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/auth.test":
			response = map[string]any{"ok": true, "user_id": "U_REP", "team_id": "T_NATIVE"}
		case "/conversations.info":
			response = map[string]any{"ok": true, "channel": map[string]any{"id": "C_NATIVE", "is_private": true, "is_member": true, "name": "Pilot"}}
		case "/conversations.members":
			response = map[string]any{"ok": true, "members": []string{"U_BOT", "U_REP"}}
		case "/users.info":
			id := r.URL.Query().Get("user")
			response = map[string]any{"ok": true, "user": map[string]any{"id": id, "team_id": "T_NATIVE", "is_bot": id == "U_BOT", "real_name": "Rita Rep", "profile": map[string]string{"email": "rep@chat.test"}}}
		case "/chat.getPermalink":
			response = map[string]any{"ok": true, "permalink": "https://fixture.slack.com/archives/C_NATIVE/p" + strings.ReplaceAll(fixture.root, ".", "")}
		case "/conversations.history":
			response = map[string]any{"ok": true, "messages": []any{map[string]any{"ts": fixture.root, "user": "U_REP", "text": fixture.source}}}
		case "/conversations.replies":
			if fixture.limited {
				w.Header().Set("Retry-After", "1800")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			if fixture.incomplete {
				if r.URL.Query().Get("cursor") == "tail" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				response = map[string]any{"ok": true, "has_more": true, "messages": []any{map[string]any{"ts": fixture.root, "user": "U_REP", "text": fixture.source}}, "response_metadata": map[string]string{"next_cursor": "tail"}}
				break
			}
			if fixture.gone {
				response = map[string]any{"ok": false, "error": "thread_not_found"}
				break
			}
			text := fixture.source
			if fixture.revised {
				text = "Legal approved the amended agreement; the pilot may now start."
			}
			response = map[string]any{"ok": true, "messages": []any{map[string]any{"ts": fixture.root, "user": "U_REP", "text": text}}}
		default:
			t.Errorf("unexpected native endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
}

func assertNativeSourceAccess(memberCtx context.Context, t *testing.T, puller *Puller, store *chatlinks.Store, original ids.UUID, expected activities.ChatSourceBinding) (context.Context, ids.ActivityID, activities.ChatSourceBinding) {
	t.Helper()
	activityID := ids.ActivityID{UUID: original}
	humanRead := memberCtx
	humanActor, _ := principal.Actor(humanRead)
	humanActor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
	humanRead = principal.WithActor(humanRead, humanActor)
	visible, err := puller.Activities.GetActivity(humanRead, activityID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if visible.SourceId != nil {
		t.Fatalf("unverified source identity exposed: %v", visible.SourceId)
	}
	encodedVisible, err := json.Marshal(visible)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"C_NATIVE", "T_NATIVE", "connection_id", "generated_fingerprint", "fixture.slack.com"} {
		if strings.Contains(string(encodedVisible), private) {
			t.Fatalf("unverified source metadata exposed: %s", private)
		}
	}
	binding, err := puller.Activities.ChatSource(humanRead, activityID)
	if err != nil {
		t.Fatal(err)
	}
	if binding != expected {
		t.Fatalf("wrong source binding: %#v", binding)
	}
	sourceURL, err := store.VerifySource(humanRead, binding.Provider, binding.Connection, binding.Channel, binding.Root)
	if err != nil || !strings.Contains(sourceURL, "/archives/C_NATIVE/p") {
		t.Fatalf("own provider source proof: %q %v", sourceURL, err)
	}
	deniedActor := humanActor
	deniedActor.Permissions.Objects = map[string]principal.ObjectGrant{}
	deniedCtx := principal.WithActor(humanRead, deniedActor)
	if _, err := puller.Activities.ChatSource(deniedCtx, activityID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("source bypassed CRM read denial: %v", err)
	}
	return humanRead, activityID, binding
}

func resumeNativeAfterOutage(ctx context.Context, t *testing.T, e *flushEnv, connection ids.UUID, puller *Puller, fixture *nativeRootSource, now *time.Time) *Puller {
	t.Helper()
	fixture.incomplete, fixture.limited = false, true
	*now = now.Add(6 * time.Minute)
	var snooze *river.JobSnoozeError
	if err := puller.Pull(ctx, connection); !errors.As(err, &snooze) || snooze.Duration != 30*time.Minute {
		t.Fatalf("rate limit lost provider backoff: %v", err)
	}
	var retry time.Time
	if err := e.owner.QueryRow(context.Background(), `SELECT retry_at FROM chat_root WHERE link_id=$1`, e.link).Scan(&retry); err != nil || !retry.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("root retry was not persisted: %v %v", retry, err)
	}
	fixture.limited = false
	*now = now.Add(24 * time.Hour)
	restarted := *puller
	restarted.Store = chatlinks.NewStore(e.db(), puller.Chats, noFlush{}, Viewers)
	return &restarted
}

func pacedSlackFixture(t *testing.T, httpClient messaging.Doer, token, workspace, member string, now *time.Time) *slack.Bot {
	t.Helper()
	profile, err := chatwire.SlackHistoryProfile("internal", *now)
	if err != nil {
		t.Fatal(err)
	}
	meter := ratelimit.NewRegistry().NewPacer(func() time.Time { return *now })
	return slack.NewBot(httpClient, token, workspace, member).WithHistoryProfile(profile, "APP_FIXTURE").WithHistoryPacing(meter, func(ctx context.Context, wait time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		*now = now.Add(wait)
		return nil
	})
}

func connectNativeRecapMember(t *testing.T, e *flushEnv, store *chatlinks.Store, httpClient messaging.Doer) context.Context {
	t.Helper()
	vault := keyvault.NewMemory()
	store.WithMemberAuthority(func(ctx context.Context, _ pgx.Tx) (context.Context, error) { return ctx, nil })
	store.WithMessenger(func() keyvault.Vault { return vault }, func(_ context.Context, grant chatlinks.MemberGrant, secret []byte) (messaging.Client, error) {
		return slack.New(httpClient, string(secret), grant.Workspace, grant.Member), nil
	})
	memberCtx := e.member()
	memberActor, _ := principal.Actor(memberCtx)
	memberActor.SeatType = principal.SeatFull
	memberCtx = principal.WithActor(memberCtx, memberActor)
	if _, err := store.ConnectMember(memberCtx, chatlinks.MemberGrant{Provider: "slack", Workspace: "T_NATIVE", Member: "U_REP"}, []byte("synthetic-personal")); err != nil {
		t.Fatal(err)
	}

	return memberCtx
}

func readNativeRecap(t *testing.T, e *flushEnv) (ids.UUID, string) {
	t.Helper()
	var id ids.UUID
	var body string
	var count int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE source_system=$1 AND source=$2`, provenance.ChatSummarySource, provenance.RecordSourceAutomation).Scan(&count); err != nil || count != 1 {
		t.Fatalf("recap count=%d error=%v", count, err)
	}
	if err := e.owner.QueryRow(context.Background(), `SELECT id,body FROM activity WHERE source_system=$1 AND source=$2`, provenance.ChatSummarySource, provenance.RecordSourceAutomation).Scan(&id, &body); err != nil {
		t.Fatal(err)
	}
	return id, body
}

func assertNativeRecapAuditBoundary(t *testing.T, e *flushEnv, activity, connection ids.UUID, root string) {
	t.Helper()
	var exposed int
	err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log
 WHERE entity_type='activity' AND entity_id=@activity
 AND (strpos(coalesce(before::text,'')||coalesce(after::text,''),@connection)>0
 OR strpos(coalesce(before::text,'')||coalesce(after::text,''),@root)>0)`,
		pgx.StrictNamedArgs{"activity": activity, "connection": connection.String(), "root": root}).Scan(&exposed)
	if err != nil {
		t.Fatal(err)
	}
	if exposed != 0 {
		t.Fatalf("recap history exposes %d internal provider source bindings", exposed)
	}
}
