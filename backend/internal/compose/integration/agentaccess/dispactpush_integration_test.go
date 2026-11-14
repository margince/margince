// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// Dispact's push receiver over the composed route. The notice rules are unit
// tests in chatsync. This suite proves the real route takes a notice with no
// session. It also proves that River's uniqueness turns a burst into one pull.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/lastseen"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/platform/replayguard"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	pushSecret = "whsec_cHVzaC1zZWNyZXQtZm9yLWl0"
	pushSlug   = "deal-acme"
)

// setupDispactPush connects a Dispact bot with an events secret and links one
// channel to a deal.
func setupDispactPush(t *testing.T) *connectorEnv {
	t.Helper()
	rdb := redistest.Client(t)
	inserter, err := jobs.NewInserter(apptest.EarlyPool(t), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	vault := keyvault.NewMemory()
	e := setupConnectorWith(t, compose.WithKeyvault(vault),
		compose.WithChatEvents(replayguard.New(rdb), lastseen.New(rdb)), compose.WithChatLinks(inserter, true, nil))
	ctx := context.Background()
	var ws, admin ids.UUID
	if err := e.Owner.QueryRow(ctx, `SELECT w.id, u.id FROM workspace w, app_user u WHERE u.email = $1`, dispactAdmin).Scan(&ws, &admin); err != nil {
		t.Fatal(err)
	}
	ref, err := vault.Put(ctx, ids.From[ids.WorkspaceKind](ws), []byte(pushSecret))
	if err != nil {
		t.Fatal(err)
	}
	connection, pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO channel_connection (id, provider, channel_id, channel_label, address, credential_ref, events_credential_ref, status, connected_by)
			VALUES ($1, 'dispact', $2, 'Margince', 'https://chat.example.test', 'vault:test', $3, 'connected', $4)`, []any{connection, dispactTeam, string(ref), admin}},
		{`INSERT INTO pipeline (id, name) VALUES ($1, 'Chat')`, []any{pipeline}},
		{`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
			VALUES ($1, 'Acme renewal', 'open', $2, $3, $4, 'seed', 'system')`, []any{deal, admin, pipeline, stage}},
		{`INSERT INTO chat_link (connection_id, provider, channel_slug, channel_name, deal_id, verbosity, linked_by)
			VALUES ($1, 'dispact', $2, 'Acme', $3, 'important', $4)`, []any{connection, pushSlug, deal, admin}},
	} {
		if _, err := e.Owner.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("%s: %v", stmt.sql, err)
		}
	}
	return e
}

// postNotice sends body signed with secret now, as Dispact does, with no
// session cookie.
func postNotice(t *testing.T, e *connectorEnv, id, secret, body string) httpResult {
	t.Helper()
	ts := time.Now().Unix()
	sig, err := webhooks.Sign(secret, id, ts, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, e.TS.URL+"/v1/channel-events/dispact", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", id)
	req.Header.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("webhook-signature", sig)
	// The server's TLS transport with no cookie jar: Dispact holds no session.
	client := &http.Client{Transport: e.TS.Client().Transport}
	resp, err := client.Do(req) //nolint:bodyclose // closed by apptest.CloseBody below
	if err != nil {
		t.Fatal(err)
	}
	defer apptest.CloseBody(t, resp)
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return httpResult{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: string(raw)}
}

func (e *connectorEnv) pullJobs(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind = 'chat_pull'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func messageNotice(team string) string {
	return fmt.Sprintf(`{"type":"message.created","workspace_id":%q,"channel_slug":%q,"seq":7,"event_id":"ev","ts":1}`, team, pushSlug)
}

func TestTenNoticesQueueOnePullWithNoSession(t *testing.T) {
	e := setupDispactPush(t)
	for i := range 10 {
		got := postNotice(t, e, fmt.Sprintf("msg_%d", i), pushSecret, messageNotice(dispactTeam))
		if got.StatusCode != http.StatusOK || !strings.Contains(got.Body, "pull_queued") {
			t.Fatalf("notice %d → %d %s; want 200 pull_queued", i, got.StatusCode, got.Body)
		}
	}
	if n := e.pullJobs(t); n != 1 {
		t.Errorf("ten notices queued %d chat_pull jobs; want one", n)
	}
}

func TestTheURLCheckIsAnsweredOverTheRoute(t *testing.T) {
	e := setupDispactPush(t)
	got := postNotice(t, e, "msg_check", pushSecret, `{"type":"url_verification","challenge":"c-77"}`)
	if got.StatusCode != http.StatusOK || strings.TrimSpace(got.Body) != `{"challenge":"c-77"}` {
		t.Errorf("the URL check → %d %s; want 200 {\"challenge\":\"c-77\"}", got.StatusCode, got.Body)
	}
}

func TestAnotherTeamsNoticeNeverPullsThisInstallationsLink(t *testing.T) {
	e := setupDispactPush(t)
	got := postNotice(t, e, "msg_other", pushSecret, messageNotice("team-someone-else"))
	if got.StatusCode != http.StatusUnauthorized {
		t.Errorf("another team's notice → %d %s; want 401", got.StatusCode, got.Body)
	}
	forged := postNotice(t, e, "msg_forged", "whsec_bm90LXRoZS1zZWNyZXQ=", messageNotice(dispactTeam))
	if forged.StatusCode != http.StatusUnauthorized {
		t.Errorf("a notice signed with another secret → %d %s; want 401", forged.StatusCode, forged.Body)
	}
	if n := e.pullJobs(t); n != 0 {
		t.Errorf("refused notices queued %d chat_pull jobs", n)
	}
}
