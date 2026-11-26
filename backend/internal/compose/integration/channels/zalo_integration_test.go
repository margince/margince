// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// A connected Zalo Official Account from webhook to filed conversation. The admin
// side runs over the real HTTP surface, the webhook over the signed route, and the
// filing over the real ingest worker and sink. Zalo is the only thing simulated.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	zaloCustomer  = "5550001"
	zaloAppID     = "1111222233334444"
	zaloTimestamp = "1764070000123"
)

func setupZalo(t *testing.T) *telegramEnv {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := apptest.EarlyPool(t)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: pool})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	inserter, err := jobs.NewInserter(pool, quiet)
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	e := apptest.SetupAppWithOptions(t,
		compose.WithChannelSurface(),
		compose.WithKeyvault(vault),
		compose.WithZaloOAEdge(inserter),
	)
	apptest.BootstrapWorkspaceSession(t, e, telegramWorkspaceName, telegramAdminEmail, "Admin")
	c := &telegramEnv{AppEnv: e, vault: vault, api: &fakeTelegramAPI{}, inserter: inserter, log: quiet}
	c.resolveActors(t)
	return c
}

type zaloOA struct{ id, oaID, secret, team string }

func (c *telegramEnv) connectOA(t *testing.T, oaID string) zaloOA {
	t.Helper()
	return c.connectOAFor(t, oaID, c.readerTeam(t).String())
}

// connectOAFor connects the OA with an existing reader team, as a reconnect of
// the same OA must.
func (c *telegramEnv) connectOAFor(t *testing.T, oaID, team string) zaloOA {
	t.Helper()
	oa := zaloOA{oaID: oaID, secret: "oa-secret-" + oaID, team: team}
	var made struct{ ID, MailVisibility, SeatID string }
	status := c.Call(t, http.MethodPost, "/v1/channel-connections/zalo-oa", integration.AnyMap{
		"oaId": oaID, "appId": zaloAppID, "displayName": "Shop " + oaID, "appSecret": "app-secret-" + oaID,
		"oaSecretKey": oa.secret, "accessToken": "access-" + oaID, "refreshToken": "refresh-" + oaID, "readerTeamId": oa.team,
	}, nil, &made)
	if status != http.StatusCreated {
		t.Fatalf("connecting OA %s → %d, want 201", oaID, status)
	}
	if made.MailVisibility != "held" || made.SeatID == "" {
		t.Fatalf("a new OA is %+v, want a held seat", made)
	}
	oa.id = made.ID
	return oa
}

func (oa zaloOA) sign(body []byte) string {
	sum := sha256.Sum256([]byte(zaloAppID + string(body) + zaloTimestamp + oa.secret))
	return "mac=" + hex.EncodeToString(sum[:])
}

func (c *telegramEnv) deliverZalo(t *testing.T, oa zaloOA, body string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.TS.URL+"/v1/channel-events/zalo-oa", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ZEvent-Signature", oa.sign([]byte(body)))
	resp, err := c.Client.Do(req) //nolint:bodyclose // closed by apptest.CloseBody below; bodyclose only recognises a Close in the same package
	if err != nil {
		t.Fatalf("delivering to the webhook: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	return resp.StatusCode
}

func zaloInbound(oaID, msgID, text string) string {
	const user = zaloCustomer
	return fmt.Sprintf(`{"app_id":%q,"event_name":"user_send_text","timestamp":%q,"sender":{"id":%q},"recipient":{"id":%q},"message":{"msg_id":%q,"text":%q}}`,
		zaloAppID, zaloTimestamp, user, oaID, msgID, text)
}

func zaloOutbound(oaID, msgID, text string) string {
	const user = zaloCustomer
	return fmt.Sprintf(`{"app_id":%q,"event_name":"oa_send_text","timestamp":%q,"sender":{"id":%q},"recipient":{"id":%q},"message":{"msg_id":%q,"text":%q}}`,
		zaloAppID, zaloTimestamp, oaID, user, msgID, text)
}

func zaloSeen(oaID string) string {
	const user = zaloCustomer
	return fmt.Sprintf(`{"app_id":%q,"event_name":"user_seen_message","timestamp":%q,"sender":{"id":%q},"recipient":{"id":%q},"message":{"msg_ids":["m1"]}}`,
		zaloAppID, zaloTimestamp, user, oaID)
}

func (c *telegramEnv) zaloActivities(t *testing.T) int {
	return c.count(t, `SELECT count(*) FROM activity WHERE source_system = 'zalo_oa'`)
}

func (c *telegramEnv) zaloRaw(t *testing.T) int {
	return c.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'zalo_oa'`)
}

func (c *telegramEnv) zaloFiled(t *testing.T, msgID string) (filedMessage, bool) {
	t.Helper()
	var f filedMessage
	var found bool
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT a.id::text, a.direction, a.audience, a.thread_key, coalesce(l.contact_id::text, '')
			  FROM activity a LEFT JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'contact'
			 WHERE a.source_system = 'zalo_oa' AND a.source_id LIKE '%:' || $1`, msgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		if rows.Next() {
			found = true
			return rows.Scan(&f.id, &f.direction, &f.audience, &f.thread, &f.contact)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the filed message: %v", err)
	}
	return f, found
}

func awaitZalo(t *testing.T, sub <-chan *river.Event) {
	t.Helper()
	awaitJobKind(t, sub, compose.ZaloOAIngestArgs{}.Kind())
}

func TestAZaloCustomerMessageAndTheOAAdminReplyFileInOneThreadOnOneContact(t *testing.T) {
	c := setupZalo(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	oa := c.connectOA(t, "9001")

	if status := c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-in-1", "Is it in stock?")); status != http.StatusOK {
		t.Fatalf("customer message → %d", status)
	}
	awaitZalo(t, sub)
	if status := c.deliverZalo(t, oa, zaloOutbound(oa.oaID, "msg-out-1", "Yes, shipping Monday.")); status != http.StatusOK {
		t.Fatalf("OA Admin reply → %d", status)
	}
	awaitZalo(t, sub)

	in, ok1 := c.zaloFiled(t, "msg-in-1")
	out, ok2 := c.zaloFiled(t, "msg-out-1")
	if !ok1 || !ok2 {
		t.Fatalf("filed customer message %v, reply %v, want both", ok1, ok2)
	}
	if in.direction != "inbound" || out.direction != "outbound" {
		t.Errorf("directions = %q and %q, want inbound and outbound", in.direction, out.direction)
	}
	if in.thread != "zalo_oa:biz:9001:5550001" || out.thread != in.thread {
		t.Errorf("threads = %q and %q, want one thread zalo_oa:biz:9001:5550001", in.thread, out.thread)
	}
	if in.contact == "" || out.contact != in.contact {
		t.Errorf("contacts = %q and %q, want one customer contact", in.contact, out.contact)
	}

	c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-in-1", "Is it in stock?"))
	if a, r, j := c.zaloActivities(t), c.zaloRaw(t), c.count(t, `SELECT count(*) FROM river_job WHERE kind = 'zalo_oa_ingest'`); a != 2 || r != 2 || j != 2 {
		t.Errorf("after a redelivery %d activities, %d raw rows and %d jobs, want 2 of each", a, r, j)
	}
}

func TestAZaloSeenEventNeverCreatesAnEntry(t *testing.T) {
	c := setupZalo(t)
	oa := c.connectOA(t, "9002")
	if status := c.deliverZalo(t, oa, zaloSeen(oa.oaID)); status != http.StatusOK {
		t.Fatalf("seen delivery → %d", status)
	}
	if a, r := c.zaloActivities(t), c.zaloRaw(t); a != 0 || r != 0 {
		t.Errorf("a seen event left %d activities and %d raw rows, want none", a, r)
	}
}

func TestTheSameZaloUserIDUnderTwoOAsIsKeyedPerOA(t *testing.T) {
	c := setupZalo(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	a, b := c.connectOA(t, "9003"), c.connectOA(t, "9004")
	c.deliverZalo(t, a, zaloInbound(a.oaID, "msg-a", "to a"))
	awaitZalo(t, sub)
	c.deliverZalo(t, b, zaloInbound(b.oaID, "msg-b", "to b"))
	awaitZalo(t, sub)
	if got := c.zaloActivities(t); got != 2 {
		t.Fatalf("%d activities, want one per OA", got)
	}
	if got := c.count(t, `SELECT count(DISTINCT s) FROM raw_capture, unnest(channel_subjects) s WHERE source_system = 'zalo_oa'`); got != 2 {
		t.Errorf("%d distinct raw subjects, want one per OA: an erasure under one OA must not reach the other", got)
	}
}

func TestAZaloDisconnectBetweenStoringAndFilingLeavesNothing(t *testing.T) {
	c := setupZalo(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	oa := c.connectOA(t, "9005")
	if status := c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-d1", "stored first")); status != http.StatusOK {
		t.Fatalf("delivery → %d", status)
	}
	if got := c.zaloRaw(t); got != 1 {
		t.Fatalf("%d raw rows stored before the disconnect, want 1", got)
	}
	if status := c.Call(t, http.MethodDelete, "/v1/channel-connections/zalo-oa/"+oa.id, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("disconnect → %d, want 204", status)
	}
	startTelegramWorker(t, runner)
	awaitZalo(t, sub)
	if a, r := c.zaloActivities(t), c.zaloRaw(t); a != 0 || r != 0 {
		t.Errorf("a disconnected OA left %d activities and %d raw rows, want none", a, r)
	}
	if got := c.count(t, `SELECT count(*) FROM capture_connection WHERE provider = 'zalo_oa' AND status = 'disconnected'`); got != 1 {
		t.Errorf("%d disconnected seats, want the OA's seat ended", got)
	}
}

func TestAZaloCustomerErasedBetweenStoringAndFilingLeavesNoOriginal(t *testing.T) {
	c := setupZalo(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	oa := c.connectOA(t, "9006")
	if status := c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-e1", "stored first")); status != http.StatusOK {
		t.Fatalf("delivery → %d", status)
	}
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO erasure_suppression (kind, value_hash) VALUES ('channel_identity', $1)`,
			storekit.ChannelIdentityHash("zalo_oa", storekit.ChannelSuppressionAccount(oa.oaID, "5550001")))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	startTelegramWorker(t, runner)
	awaitZalo(t, sub)
	if a, r := c.zaloActivities(t), c.zaloRaw(t); a != 0 || r != 0 {
		t.Errorf("an erased customer left %d activities and %d raw rows, want none", a, r)
	}
}

func TestAHeldZaloOAKeepsMessagesFromColleaguesAndSharingOpensTheNextOne(t *testing.T) {
	c := setupZalo(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	oa := c.connectOA(t, "9007")
	path := "/v1/channel-connections/zalo-oa/" + oa.id + "/visibility"

	c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-h1", "held"))
	awaitZalo(t, sub)
	if f, _ := c.zaloFiled(t, "msg-h1"); f.audience != "participants" {
		t.Fatalf("a message on a held OA is %q, want participants", f.audience)
	}
	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "shared", "confirm": true}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("shared without the workspace opt-in → %d, want 422", status)
	}
	c.allowSharedPosture(t)
	var changed struct{ MailVisibility string }
	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "shared", "confirm": true}, nil, &changed); status != http.StatusOK || changed.MailVisibility != "shared" {
		t.Fatalf("sharing → %d %q, want 200 shared", status, changed.MailVisibility)
	}
	c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-h2", "shared"))
	awaitZalo(t, sub)
	if f, _ := c.zaloFiled(t, "msg-h2"); f.audience != "workspace" {
		t.Errorf("the next message is %q, want workspace", f.audience)
	}
	if f, _ := c.zaloFiled(t, "msg-h1"); f.audience != "participants" {
		t.Errorf("sharing published the earlier held message: %q", f.audience)
	}
}

func TestABusinessThreadOnZaloCannotBeRepliedTo(t *testing.T) {
	c := setupZalo(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	oa := c.connectOA(t, "9008")
	c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-r1", "question"))
	awaitZalo(t, sub)
	filed, _ := c.zaloFiled(t, "msg-r1")
	c.grantConsent(t, filed.contact, "transactional")

	status := c.Call(t, http.MethodPost, "/v1/activities/"+filed.id+"/send-message", integration.AnyMap{
		"body": "reply from the CRM", "consent_purpose": "transactional",
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a reply into a Zalo business thread → %d, want 422", status)
	}
	if got := c.count(t, `SELECT count(*) FROM activity WHERE direction = 'outbound' AND thread_key = $1 AND source_id NOT LIKE '%:msg-%'`, filed.thread); got != 0 {
		t.Errorf("%d outbound activities staged on the thread, want none", got)
	}
}

func TestErasingAZaloContactDestroysItsRawOriginals(t *testing.T) {
	c := setupZalo(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	oa := c.connectOA(t, "9009")
	c.deliverZalo(t, oa, zaloInbound(oa.oaID, "msg-p1", "filed message"))
	awaitZalo(t, sub)
	filed, _ := c.zaloFiled(t, "msg-p1")
	if got := c.zaloRaw(t); got != 1 {
		t.Fatalf("%d raw rows before the erasure, want the one filed", got)
	}
	contact, err := ids.Parse(filed.contact)
	if err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(c.DB()).EraseContact(c.eraserCtx(t), contact, "test"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
	if got := c.zaloRaw(t); got != 0 {
		t.Errorf("%d Zalo raw originals survive the erasure, want none", got)
	}
}

// allowSharedPosture opts the workspace in to seats that share what they file.
func (c *telegramEnv) allowSharedPosture(t *testing.T) {
	t.Helper()
	on := true
	settings := capture.NewSettings(compose.NewSettingsStore(c.Pool), compose.StoredConfig(c.Pool))
	actor, _ := principal.Actor(c.adminCtx(t))
	actor.Permissions = integration.AdminPerms
	if _, err := settings.Update(principal.WithActor(c.adminCtx(t), actor), capture.SettingsPatch{SharedPostureAllowed: &on}); err != nil {
		t.Fatalf("opting the workspace in: %v", err)
	}
}
