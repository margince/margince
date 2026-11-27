// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package zaloedge_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/zaloedge"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	appID     = "1111222233334444"
	timestamp = "1764070000123"
)

type env struct {
	owner    *pgx.Conn
	db       *database.DB
	pool     *pgxpool.Pool
	ws       ids.UUID
	vault    keyvault.Vault
	store    *capture.ChannelStore
	handlers capture.ZaloEventHandlers
	admin    context.Context
	rep      context.Context
	team     ids.UUID
	anon     context.Context
}

func human(ctx context.Context, ws, user ids.UUID, grant principal.ObjectGrant) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"role"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"channel_connection": grant},
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func setup(t *testing.T) *env {
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
	e := &env{owner: owner, ws: ids.NewV7(), vault: keyvault.NewMemory(), team: ids.NewV7()}
	adminID, repID := ids.NewV7(), ids.NewV7()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace (id) VALUES ($1)`, []any{e.ws}},
		{`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'admin@zalo.test', 'Admin')`, []any{adminID}},
		{`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rep@zalo.test', 'Rep')`, []any{repID}},
		{`INSERT INTO team (id, name) VALUES ($1, 'Front desk')`, []any{e.team}},
		{`INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, []any{e.team, adminID}},
	} {
		if _, err := owner.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if e.pool, err = testdb.Pool(ctx, appDSN); err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	e.db = database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	e.store = capture.NewChannelStore(e.db, e.vault, telegram.NewAPI(nil, ""), nil)
	inserter, err := jobs.NewInserter(e.pool, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	e.handlers = capture.NewZaloEventHandlers(&zaloedge.Receiver{
		Endpoints: capture.NewZaloEdge(e.db, func() keyvault.Vault { return e.vault }, slog.Default()),
		DB:        e.db,
		Inserter:  inserter,
		Job: func(ws, connection, raw ids.UUID) river.JobArgs {
			return compose.ZaloOAIngestArgs{ConnectionID: connection, RawCaptureID: raw}
		},
		MaxAttempts: 3,
		Log:         slog.Default(),
	})
	e.admin = human(ctx, e.ws, adminID, principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true})
	e.rep = human(ctx, e.ws, repID, principal.ObjectGrant{Read: true})
	e.anon = principal.WithWorkspaceID(ctx, e.ws)
	return e
}

type account struct {
	oaID, secret string
	conn         capture.ZaloOAConnection
}

func (e *env) connect(t *testing.T, oaID, secret string) account {
	t.Helper()
	conn, err := e.store.ConnectZaloOA(e.admin, capture.ConnectZaloOARequest{
		OAID: oaID, AppID: appID, DisplayName: "OA " + oaID, AppSecret: "app-secret-" + oaID, OASecretKey: secret,
		AccessToken: "access-" + oaID, RefreshToken: "refresh-" + oaID, ReaderTeam: e.team,
	})
	if err != nil {
		t.Fatalf("ConnectZaloOA(%s): %v", oaID, err)
	}
	return account{oaID: oaID, secret: secret, conn: conn}
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) rawRows(t *testing.T) int {
	return e.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'zalo_oa'`)
}

func (e *env) ingestJobs(t *testing.T) int {
	return e.count(t, `SELECT count(*) FROM river_job WHERE kind = 'zalo_oa_ingest' AND args->>'workspace_id' = $1`, e.ws.String())
}

func sign(secret string, body []byte) string { return signFor(appID, secret, body) }

func signFor(app, secret string, body []byte) string {
	sum := sha256.Sum256([]byte(app + string(body) + timestamp + secret))
	return "mac=" + hex.EncodeToString(sum[:])
}

func inbound(oaID, user, msgID, text string) []byte {
	return []byte(fmt.Sprintf(`{"app_id":%q,"event_name":"user_send_text","timestamp":%q,"sender":{"id":%q},"recipient":{"id":%q},"message":{"msg_id":%q,"text":%q}}`,
		appID, timestamp, user, oaID, msgID, text))
}

func (e *env) post(body []byte, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/channel-events/zalo-oa", strings.NewReader(string(body))).WithContext(e.anon)
	if signature != "" {
		req.Header.Set("X-ZEvent-Signature", signature)
	}
	rec := httptest.NewRecorder()
	e.handlers.ReceiveZaloOAWebhook(rec, req)
	return rec
}
