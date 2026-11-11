// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gdocs"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const notesText = "Summary\nAda will send the offer by Friday.\n"

// docsFeed is a stubbed Google Docs API. Each Doc answers its text, or the
// error a test sets for it, and every read is counted.
type docsFeed struct {
	mu      sync.Mutex
	refused bool
	reads   []string
}

func (f *docsFeed) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reads)
}

func (f *docsFeed) serve(t *testing.T, account string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{
			"access_token": "docs-access", "refresh_token": "docs-refresh", "expires_in": 3599,
			"scope": strings.Join(gdocs.ConsentScopes(), " "),
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"email": account})
	})
	mux.HandleFunc("/v1/documents/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/documents/")
		f.mu.Lock()
		f.reads = append(f.reads, id)
		refused := f.refused
		f.mu.Unlock()
		if refused {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			integration.WriteJSON(w, map[string]any{"error": map[string]any{
				"code": 403, "status": "PERMISSION_DENIED",
				"details": []any{map[string]any{"reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT"}},
			}})
			return
		}
		integration.WriteJSON(w, map[string]any{
			"title": "Kick-off review - Notes by Gemini", "revisionId": "rev-" + id,
			"tabs": []any{map[string]any{"documentTab": map[string]any{"body": map[string]any{"content": []any{
				map[string]any{"paragraph": map[string]any{"elements": []any{
					map[string]any{"textRun": map[string]any{"content": notesText}},
				}}},
			}}}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// queuedReadings counts the readings the landing queued.
type queuedReadings struct {
	mu       sync.Mutex
	activity []ids.ActivityID
}

func (q *queuedReadings) enqueue(_ context.Context, _ pgx.Tx, read activities.TranscriptRead) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.activity = append(q.activity, read.ActivityID)
	return nil
}

// connectNotes connects Rep1's Google Docs grant on account, over a registry
// built the way production builds it, and answers a sync of it.
func connectNotes(t *testing.T, env captureEnv, feed *docsFeed, account string, queued *queuedReadings) (ids.UUID, func()) {
	t.Helper()
	stub := feed.serve(t, account)
	oauth := gdocs.NewOAuth(gdocs.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	conn := gdocs.New(oauth, gdocs.NewOwner(stub.Client(), stub.URL)).WithDocs(gdocs.NewDocsAPI(stub.Client(), stub.URL))
	reg := compose.NewCaptureRegistry(env.e.Pool, newTestKeyvault(t, env.e), compose.CaptureConfig{
		Now: func() time.Time { return captureNow }, TranscriptEnqueue: queued.enqueue,
	})
	reg.Register(conn)
	authReq, err := gdocs.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gdocs/callback")
	if err != nil {
		t.Fatal(err)
	}
	granted, err := conn.Authenticate(context.Background(), authReq)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	connID, err := reg.Connect(humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead}), "gdocs", granted)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return connID, func() {
		t.Helper()
		if err := reg.SyncOnce(principal.WithWorkspaceID(context.Background(), env.e.WS), connID); err != nil {
			t.Fatalf("SyncOnce gdocs: %v", err)
		}
	}
}

// meetingWithNotes syncs Rep1's calendar once with a Meet call carrying its
// notes Doc, and answers the meeting and the feed to change it.
func meetingWithNotes(t *testing.T, env captureEnv) (ids.UUID, *calendarFeed, func()) {
	t.Helper()
	seatFor(t, env.e, guestColleagueA)
	ensureGuestContact(t, env.e, guestKnown)
	feed := &calendarFeed{}
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...),
		driveFile(notesFileID, "Kick-off review - Notes by Gemini")))
	sync := connectFeed(t, env, feed)
	sync()
	return gcalMeeting(t, env.e), feed, sync
}

func notesURL(fileID string) string {
	return "https://docs.google.com/document/d/" + fileID + "/edit"
}

// transcriptsOf counts the transcripts landed from one Doc.
func transcriptsOf(t *testing.T, e *integration.SearchEnv, fileID string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'transcript' AND source_id = '`+notesURL(fileID)+`'`)
}

// A meeting's notes Doc is read through the owner's Docs grant and lands once,
// as a transcript of that meeting through the activity writer, which queues
// its reading. A second sync opens no Doc again.
func TestMeetingNotesLandOnceAsATranscriptAndQueueAReading(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, _, _ := meetingWithNotes(t, env)
	docs, queued := &docsFeed{}, &queuedReadings{}
	_, syncNotes := connectNotes(t, env, docs, captureOwner, queued)

	syncNotes()
	syncNotes()

	if got := docs.readCount(); got != 1 {
		t.Errorf("the notes Doc was opened %d times over two syncs, want once", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 1 {
		t.Fatalf("%d transcripts from the notes Doc, want 1", got)
	}
	var (
		transcript ids.ActivityID
		body, kind string
		from       string
		capturedBy string
	)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT id, body, kind, raw -> 'google_doc' ->> 'meeting_id', captured_by FROM activity
			 WHERE source_system = 'transcript' AND source_id = $1`, notesURL(notesFileID)).
			Scan(&transcript, &body, &kind, &from, &capturedBy)
	}); err != nil {
		t.Fatal(err)
	}
	if body != strings.TrimSuffix(notesText, "\n") || kind != "meeting" || from != meeting.String() {
		t.Errorf("transcript: body %q kind %q from %q, want the notes text on a meeting read from %s", body, kind, from, meeting)
	}
	if !strings.HasPrefix(capturedBy, "connector:") {
		t.Errorf("transcript captured_by = %q, want the connector principal", capturedBy)
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+transcript.String()+`' AND action = 'create'`); got != 1 {
		t.Errorf("%d create audits on the transcript, want 1", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = '`+transcript.String()+`'`); got != 1 {
		t.Errorf("%d events announced the transcript, want 1", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM transcript_read WHERE activity_id = '`+transcript.String()+`' AND status = 'queued'`); got != 1 {
		t.Errorf("%d queued readings of the transcript, want 1", got)
	}
	if !slices.Equal(queued.activity, []ids.ActivityID{transcript}) {
		t.Errorf("readings enqueued for %v, want the transcript once", queued.activity)
	}
}

// A Doc taken off the event is archived from the meeting at the next calendar
// sync, and the notes reader never opens it.
func TestANotesDocTakenOffTheEventIsNotRead(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, feed, syncCalendar := meetingWithNotes(t, env)
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...)))
	syncCalendar()

	if got := countRows(t, e, `SELECT count(*) FROM attachment WHERE entity_id = '`+meeting.String()+
		`' AND category = 'link' AND archived_at IS NOT NULL`); got != 1 {
		t.Fatalf("%d archived notes links on the meeting, want the removed Doc", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+meeting.String()+
		`' AND (after ->> 'meeting_documents_removed')::int = 1`); got != 1 {
		t.Errorf("%d meeting audits record the removed Doc, want 1", got)
	}

	docs := &docsFeed{}
	_, syncNotes := connectNotes(t, env, docs, captureOwner, &queuedReadings{})
	syncNotes()

	if got := docs.readCount(); got != 0 {
		t.Errorf("a Doc taken off the event was opened %d times, want never", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 0 {
		t.Errorf("%d transcripts from a removed Doc, want 0", got)
	}
}

// A Docs grant on another Google account than the calendar's reads nothing:
// notes are read only for the meetings of the account that granted them.
func TestANotesGrantOnAnotherAccountReadsNothing(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meetingWithNotes(t, env)
	docs := &docsFeed{}
	_, syncNotes := connectNotes(t, env, docs, "owner.private@myco.example", &queuedReadings{})
	syncNotes()

	if got := docs.readCount(); got != 0 {
		t.Errorf("a grant on another account opened %d Docs, want none", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 0 {
		t.Errorf("%d transcripts through another account, want 0", got)
	}
}

// Google refusing the read grant takes it off the connection's recorded
// scopes, which is what Connections reads to say the row cannot read notes.
// The next sync opens nothing until the owner connects again.
func TestARefusedNotesGrantAsksTheOwnerToReconnect(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meetingWithNotes(t, env)
	docs := &docsFeed{refused: true}
	connID, syncNotes := connectNotes(t, env, docs, captureOwner, &queuedReadings{})
	syncNotes()

	var scopes []string
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT provider_scopes, status FROM capture_connection WHERE id = $1`, connID).Scan(&scopes, &status)
	}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(scopes, gdocs.ReadScope) {
		t.Errorf("recorded scopes %v still hold the refused read grant", scopes)
	}
	if !slices.Contains(scopes, gdocs.EmailScope) {
		t.Errorf("recorded scopes %v lost a grant Google did not refuse", scopes)
	}
	if status != "connected" {
		t.Errorf("connection status %q, want connected: the account still syncs", status)
	}
	syncNotes()
	if got := docs.readCount(); got != 1 {
		t.Errorf("%d Doc reads after the refusal, want only the refused one", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 0 {
		t.Errorf("%d transcripts after a refused grant, want 0", got)
	}
}
