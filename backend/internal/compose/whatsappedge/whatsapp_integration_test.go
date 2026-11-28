// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package whatsappedge_test

// The WhatsApp connection and webhook edge over a real Postgres. An admin
// connects numbers and Meta's calls are checked against the sealed secrets.
// Every accepted item is one raw_capture row and one ingest job.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/whatsappedge"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type env struct {
	owner    *pgx.Conn
	team     ids.UUID
	db       *database.DB
	pool     *pgxpool.Pool
	ws       ids.UUID
	vault    keyvault.Vault
	store    *capture.ChannelStore
	handlers capture.WhatsAppEventHandlers
	admin    context.Context
	rep      context.Context
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
	e := &env{owner: owner, ws: ids.NewV7(), team: ids.NewV7(), vault: keyvault.NewMemory()}
	adminID, repID := ids.NewV7(), ids.NewV7()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace (id) VALUES ($1)`, []any{e.ws}},
		{`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'admin@wa.test', 'Admin')`, []any{adminID}},
		{`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rep@wa.test', 'Rep')`, []any{repID}},
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
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	e.db = db
	e.store = capture.NewChannelStore(db, e.vault, telegram.NewAPI(nil, ""), nil)
	inserter, err := jobs.NewInserter(e.pool, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	e.handlers = capture.NewWhatsAppEventHandlers(&whatsappedge.Receiver{
		Endpoints: capture.NewWhatsAppEdge(db, func() keyvault.Vault { return e.vault }, slog.Default()),
		DB:        db,
		Inserter:  inserter,
		Job: func(ws, connection, raw ids.UUID) river.JobArgs {
			return compose.WhatsAppIngestArgs{ConnectionID: connection, RawCaptureID: raw}
		},
		MaxAttempts: 3,
		Log:         slog.Default(),
	})
	e.admin = human(ctx, e.ws, adminID, principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true})
	e.rep = human(ctx, e.ws, repID, principal.ObjectGrant{Read: true})
	e.anon = principal.WithWorkspaceID(ctx, e.ws)
	return e
}

type number struct {
	phoneID, secret string
	conn            capture.WhatsAppConnection
	verifyToken     string
}

func (e *env) connect(t *testing.T, phoneID, secret string) number {
	t.Helper()
	return e.connectUnder(t, phoneID, secret, "waba")
}

func (e *env) connectUnder(t *testing.T, phoneID, secret, waba string) number {
	t.Helper()
	conn, verify, err := e.store.ConnectWhatsApp(e.admin, capture.ConnectWhatsAppRequest{
		WABAID: waba, PhoneNumberID: phoneID, DisplayPhone: "+49 30 " + phoneID,
		AccessToken: "EAAG-token-" + phoneID, AppSecret: secret, ReaderTeam: e.team,
	})
	if err != nil {
		t.Fatalf("ConnectWhatsApp(%s): %v", phoneID, err)
	}
	return number{phoneID: phoneID, secret: secret, conn: conn, verifyToken: verify}
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
	return e.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'whatsapp'`)
}

func (e *env) ingestJobs(t *testing.T) int {
	return e.count(t, `SELECT count(*) FROM river_job WHERE kind = 'whatsapp_ingest'`)
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func msg(id, text string) string {
	return fmt.Sprintf(`{"from":"491701234567","id":%q,"timestamp":"1764000300","type":"text","text":{"body":%q}}`, id, text)
}

// batch is one delivery whose changes each carry the given message items for one number.
func batch(changes ...string) []byte {
	return []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba","changes":[` + strings.Join(changes, ",") + `]}]}`)
}

func change(phoneID string, items ...string) string {
	return fmt.Sprintf(`{"field":"messages","value":{"messaging_product":"whatsapp","metadata":{"display_phone_number":"1","phone_number_id":%q},`+
		`"contacts":[{"profile":{"name":"Ada"},"wa_id":"491701234567"}],"messages":[%s]}}`, phoneID, strings.Join(items, ","))
}

func (e *env) post(body []byte, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/channel-events/whatsapp", strings.NewReader(string(body))).WithContext(e.anon)
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rec := httptest.NewRecorder()
	e.handlers.ReceiveWhatsAppWebhook(rec, req)
	return rec
}

func (e *env) challenge(token string) *httptest.ResponseRecorder {
	q := url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {token}, "hub.challenge": {"abc123"}}
	req := httptest.NewRequest(http.MethodGet, "/v1/channel-events/whatsapp?"+q.Encode(), nil).WithContext(e.anon)
	rec := httptest.NewRecorder()
	e.handlers.VerifyWhatsAppWebhook(rec, req, crmcontracts.VerifyWhatsAppWebhookParams{})
	return rec
}

func TestAnAdminConnectsANumberAndARepCannot(t *testing.T) {
	e := setup(t)
	req := capture.ConnectWhatsAppRequest{WABAID: "w1", PhoneNumberID: "p1", DisplayPhone: "+49 30 1", AccessToken: "EAAG-secret-token", AppSecret: "app-secret-1", ReaderTeam: e.team}

	if _, _, err := e.store.ConnectWhatsApp(e.rep, req); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep connecting a number got %v, want a permission denial", err)
	}
	if n := e.count(t, `SELECT count(*) FROM channel_connection WHERE provider = 'whatsapp'`); n != 0 {
		t.Fatalf("%d rows after a refused connect, want none", n)
	}

	conn, verify, err := e.store.ConnectWhatsApp(e.admin, req)
	if err != nil {
		t.Fatal(err)
	}
	var provider, accountID, channelID, status string
	var approved bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT provider, account_id, channel_id, status, approved_at IS NOT NULL FROM channel_connection WHERE id = $1`, conn.ID).
		Scan(&provider, &accountID, &channelID, &status, &approved); err != nil {
		t.Fatal(err)
	}
	if provider != "whatsapp" || accountID != "p1" || channelID != "w1" || status != "connected" || !approved {
		t.Errorf("row = %s/%s/%s/%s approved=%v, want a connected, approved whatsapp row for number p1 of account w1", provider, accountID, channelID, status, approved)
	}
	if verify == "" {
		t.Error("no verify token returned")
	}
	if got := e.count(t, `SELECT count(*) FROM capture_connection s JOIN channel_connection c ON c.seat_id = s.id
		 WHERE c.id = $1 AND s.provider = 'whatsapp' AND s.user_id IS NULL AND s.reader_team_id = $2 AND s.mail_posture = 'held'`, conn.ID, e.team); got != 1 {
		t.Errorf("the connection has %d held team seats read by the reader team, want 1", got)
	}
	noTeam := req
	noTeam.PhoneNumberID = "p1b"
	noTeam.ReaderTeam = ids.Nil
	var teamErr *capture.ReaderTeamError
	if _, _, err := e.store.ConnectWhatsApp(e.admin, noTeam); !errors.As(err, &teamErr) {
		t.Errorf("connecting with no reader team got %v, want a reader-team refusal", err)
	}
	if got := e.count(t, `SELECT count(*) FROM channel_connection WHERE account_id = 'p1b'`); got != 0 {
		t.Errorf("a refused connect left %d rows", got)
	}
	if _, _, err := e.store.ConnectWhatsApp(e.admin, req); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("connecting the same number twice got %v, want a conflict", err)
	}
	if n := e.vault.(interface{ Count() int }).Count(); n != 2 {
		t.Errorf("the vault holds %d secrets, want 2: a refused connect must leave none", n)
	}
}

func TestSecretsAreSealedAndNeverReturnedOrAudited(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p2", "app-secret-2")

	var tokenRef, eventsRef string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT credential_ref, events_credential_ref FROM channel_connection WHERE id = $1`, n.conn.ID).Scan(&tokenRef, &eventsRef); err != nil {
		t.Fatal(err)
	}
	sealed, err := e.vault.Get(context.Background(), ids.From[ids.WorkspaceKind](e.ws), keyvault.Ref(tokenRef))
	if err != nil || string(sealed) != "EAAG-token-p2" {
		t.Fatalf("the access token in the vault = %q, %v", sealed, err)
	}
	list, err := e.store.ListWhatsAppConnections(e.admin)
	if err != nil || len(list) != 1 || list[0].PhoneNumberID != "p2" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if _, err := e.store.ListWhatsAppConnections(e.rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep listing connections got %v, want a permission denial", err)
	}
	secrets := []string{"EAAG-token-p2", "app-secret-2", n.verifyToken}
	var audit string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(string_agg(before::text || after::text, ' '), '') FROM audit_log WHERE entity_type = 'channel_connection'`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	var row string
	if err := e.owner.QueryRow(context.Background(), `SELECT row_to_json(c)::text FROM channel_connection c WHERE id = $1`, n.conn.ID).Scan(&row); err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		if strings.Contains(audit, secret) || strings.Contains(row, secret) || strings.Contains(fmt.Sprintf("%+v", list), secret) {
			t.Errorf("secret %q reachable outside the vault", secret)
		}
	}
}

func TestTheChallengeNeedsAConnectedNumbersVerifyToken(t *testing.T) {
	e := setup(t)
	if rec := e.challenge("anything"); rec.Code != http.StatusForbidden {
		t.Errorf("a challenge with no number connected got %d, want 403", rec.Code)
	}
	n := e.connect(t, "p3", "app-secret-3")
	rec := e.challenge(n.verifyToken)
	if rec.Code != http.StatusOK || rec.Body.String() != "abc123" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("the right token got %d %q (%s), want 200 abc123 as text/plain", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if rec := e.challenge("wrong"); rec.Code != http.StatusForbidden {
		t.Errorf("the wrong token got %d, want 403", rec.Code)
	}
	if rec := e.challenge(""); rec.Code != http.StatusForbidden {
		t.Errorf("an empty token got %d, want 403", rec.Code)
	}
}

func TestABadSignatureStoresNothing(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p4", "app-secret-4")
	body := batch(change(n.phoneID, msg("wamid.A", "hello")))

	for name, signature := range map[string]string{"none": "", "wrong secret": sign("other", body), "tampered": sign(n.secret, append([]byte(nil), body[1:]...))} {
		if rec := e.post(body, signature); rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, rec.Code)
		}
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("a refused delivery left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestASignedDeliveryStoresOneRowAndOneJobPerItem(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p5", "app-secret-5")
	body := batch(change(n.phoneID, msg("wamid.A", "one"), msg("wamid.B", "two")))

	rec := e.post(body, sign(n.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":2`) {
		t.Fatalf("status %d body %s, want 200 with two stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 2 || e.ingestJobs(t) != 2 {
		t.Fatalf("%d raw rows and %d jobs, want 2 and 2", e.rawRows(t), e.ingestJobs(t))
	}
	if again := e.post(body, sign(n.secret, body)); again.Code != http.StatusOK {
		t.Fatalf("redelivery status %d, want 200", again.Code)
	}
	if e.rawRows(t) != 2 || e.ingestJobs(t) != 2 {
		t.Errorf("after a redelivery %d raw rows and %d jobs, want 2 and 2", e.rawRows(t), e.ingestJobs(t))
	}
	var payload string
	if err := e.owner.QueryRow(context.Background(), `SELECT payload::text FROM raw_capture WHERE source_system = 'whatsapp' ORDER BY source_id LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"wamid.A"`) || strings.Contains(payload, "wamid.B") {
		t.Errorf("stored item = %s, want exactly the one message", payload)
	}
}

func TestAnUnknownNumberStoresNothing(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p6", "app-secret-6")
	body := batch(change("someone-elses-number", msg("wamid.X", "stray")))

	rec := e.post(body, sign(n.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Errorf("status %d body %s, want 200 with none stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("an unknown number left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestASecretCoversOnlyItsOwnNumbers(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "p7", "app-secret-7")
	b := e.connect(t, "p8", "app-secret-8")
	body := batch(change(a.phoneID, msg("wamid.FA", "for a")), change(b.phoneID, msg("wamid.FB", "for b")))

	if rec := e.post(body, sign(a.secret, body)); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var sources []string
	rows, err := e.owner.Query(context.Background(), `SELECT source_id FROM raw_capture WHERE source_system = 'whatsapp'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, s)
	}
	if len(sources) != 1 || !strings.HasPrefix(sources[0], "p7|") {
		t.Errorf("stored %v, want only the item of number p7, whose app secret signed the delivery", sources)
	}
}

func TestAMalformedItemDoesNotFailTheBatch(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p9", "app-secret-9")
	body := batch(change(n.phoneID, msg("wamid.G1", "good"), `"not an object"`, `{"id":"wamid.NOTIME","type":"text"}`, msg("wamid.G2", "also good")))

	rec := e.post(body, sign(n.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":2`) || !strings.Contains(rec.Body.String(), `"skipped":2`) {
		t.Fatalf("status %d body %s, want 200 with two stored and two skipped", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 2 || e.ingestJobs(t) != 2 {
		t.Errorf("%d raw rows and %d jobs, want 2 and 2", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestDisconnectStopsAcceptanceAndDestroysTheSecrets(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p10", "app-secret-10")
	body := batch(change(n.phoneID, msg("wamid.D1", "before")))
	if rec := e.post(body, sign(n.secret, body)); rec.Code != http.StatusOK || e.rawRows(t) != 1 {
		t.Fatalf("status %d, %d rows before the disconnect", rec.Code, e.rawRows(t))
	}

	if err := e.store.DisconnectWhatsApp(e.rep, n.conn.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep disconnecting got %v, want a permission denial", err)
	}
	if err := e.store.DisconnectWhatsApp(e.admin, n.conn.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.store.DisconnectWhatsApp(e.admin, n.conn.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("disconnecting twice got %v, want not found", err)
	}
	after := batch(change(n.phoneID, msg("wamid.D2", "after")))
	if rec := e.post(after, sign(n.secret, after)); rec.Code != http.StatusForbidden {
		t.Errorf("a delivery after the disconnect got %d, want 403", rec.Code)
	}
	if rec := e.challenge(n.verifyToken); rec.Code != http.StatusForbidden {
		t.Errorf("a challenge after the disconnect got %d, want 403", rec.Code)
	}
	if e.rawRows(t) != 1 {
		t.Errorf("%d raw rows after the disconnect, want the one filed before it", e.rawRows(t))
	}
	if left := e.vault.(interface{ Count() int }).Count(); left != 0 {
		t.Errorf("the vault still holds %d secrets, want none", left)
	}
	if _, _, err := e.store.ConnectWhatsApp(e.admin, capture.ConnectWhatsAppRequest{
		WABAID: "w", PhoneNumberID: n.phoneID, DisplayPhone: "d", AccessToken: "t", AppSecret: "s", ReaderTeam: e.team,
	}); err != nil {
		t.Errorf("connecting the number again after a disconnect: %v", err)
	}
}

// suppress puts an identity on the erasure list the way the eraser keys it.
func (e *env) suppress(t *testing.T, account string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO erasure_suppression (kind, value_hash) VALUES ('channel_identity', $1)`,
		storekit.ChannelIdentityHash("whatsapp", account)); err != nil {
		t.Fatal(err)
	}
}

func bsuidMsg(id, bsuid string) string {
	return fmt.Sprintf(`{"from_user_id":%q,"id":%q,"timestamp":"1764000300","type":"text","text":{"body":"hi"}}`, bsuid, id)
}

func TestAnErasedPhoneIsNotStored(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p12", "app-secret-12")
	e.suppress(t, "491701234567")
	body := batch(change(n.phoneID, msg("wamid.E1", "from an erased customer")))

	rec := e.post(body, sign(n.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) || !strings.Contains(rec.Body.String(), `"skipped":1`) {
		t.Errorf("status %d body %s, want 200 with none stored and one skipped", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("an erased phone left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestAnErasedBSUIDIsScopedToItsAccount(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p13", "app-secret-13")
	e.suppress(t, storekit.ChannelSuppressionAccount("waba", "BSUID.1"))
	e.suppress(t, "BSUID.2")
	body := batch(change(n.phoneID, bsuidMsg("wamid.B1", "BSUID.1"), bsuidMsg("wamid.B2", "BSUID.2")))

	if rec := e.post(body, sign(n.secret, body)); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var sources []string
	rows, err := e.owner.Query(context.Background(), `SELECT source_id FROM raw_capture WHERE source_system = 'whatsapp'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, s)
	}
	if len(sources) != 1 || !strings.Contains(sources[0], "wamid.B2") {
		t.Errorf("stored %v, want only BSUID.2: BSUID.1 is erased under this account, and an erasure of BSUID.2 under no account does not reach it", sources)
	}
}

func TestErasingAScopedBSUIDContactStopsItsDeliveriesUnderThatAccountOnly(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p14", "app-secret-14")
	contact := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Erased Customer', 'manual', 'human:x')`, contact); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact_channel_identity (contact_id, provider, account_scope, channel_user_id, source, captured_by)
		 VALUES ($1, 'whatsapp', 'waba', 'BSUID.9', 'channel', 'human:x')`, contact); err != nil {
		t.Fatal(err)
	}
	eraser := ctxWithContactDelete(e.admin)
	if err := privacy.NewEraser(e.db).EraseContact(eraser, contact, "test"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}

	n2 := e.connectUnder(t, "p14b", "app-secret-14b", "other-waba")
	other := []byte(strings.Replace(string(batch(change(n2.phoneID, bsuidMsg("wamid.O1", "BSUID.9")))), `"id":"waba"`, `"id":"other-waba"`, 1))
	same := batch(change(n.phoneID, bsuidMsg("wamid.S1", "BSUID.9")))
	if rec := e.post(same, sign(n.secret, same)); !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Errorf("the erased BSUID under its own account: %s, want none stored", rec.Body.String())
	}
	if rec := e.post(other, sign(n2.secret, other)); !strings.Contains(rec.Body.String(), `"stored":1`) {
		t.Errorf("the same BSUID under another account: %s, want it stored", rec.Body.String())
	}
}

func ctxWithContactDelete(ctx context.Context) context.Context {
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["contact"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return principal.WithActor(ctx, actor)
}

// disconnectingEndpoints disconnects the number right after the delivery's
// signature has been checked against it, the moment the old code read liveness.
type disconnectingEndpoints struct {
	*capture.WhatsAppEdge
	disconnect func()
}

func (d disconnectingEndpoints) LiveEndpoints(ctx context.Context) ([]capture.WhatsAppEndpoint, error) {
	endpoints, err := d.WhatsAppEdge.LiveEndpoints(ctx)
	d.disconnect()
	return endpoints, err
}

func TestANumberDisconnectedAfterVerificationStoresNothing(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p20", "app-secret-20")
	inserter, err := jobs.NewInserter(e.pool, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	e.handlers = capture.NewWhatsAppEventHandlers(&whatsappedge.Receiver{
		Endpoints: disconnectingEndpoints{
			WhatsAppEdge: capture.NewWhatsAppEdge(db, func() keyvault.Vault { return e.vault }, slog.Default()),
			disconnect: func() {
				if err := e.store.DisconnectWhatsApp(e.admin, n.conn.ID); err != nil {
					t.Errorf("disconnecting: %v", err)
				}
			},
		},
		DB: db, Inserter: inserter, MaxAttempts: 3, Log: slog.Default(),
		Job: func(ws, connection, raw ids.UUID) river.JobArgs {
			return compose.WhatsAppIngestArgs{ConnectionID: connection, RawCaptureID: raw}
		},
	})
	body := batch(change(n.phoneID, msg("wamid.L1", "late")))

	rec := e.post(body, sign(n.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Errorf("status %d body %s, want 200 with none stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("a number disconnected after verification left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestAnItemNamingAnotherBusinessAccountStoresNothing(t *testing.T) {
	e := setup(t)
	n := e.connect(t, "p21", "app-secret-21")
	body := []byte(strings.Replace(string(batch(change(n.phoneID, msg("wamid.W1", "wrong account")))), `"id":"waba"`, `"id":"another-waba"`, 1))

	rec := e.post(body, sign(n.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Errorf("status %d body %s, want 200 with none stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("an item of another business account left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}
