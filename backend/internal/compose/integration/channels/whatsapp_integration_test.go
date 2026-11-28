// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// A connected WhatsApp number from webhook to filed conversation. The admin side
// runs over the real HTTP surface, the webhook over the signed route, and the
// filing over the real ingest worker and sink. Meta is the only thing simulated.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func setupWhatsApp(t *testing.T) *telegramEnv {
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
		compose.WithWhatsAppEdge(inserter),
	)
	apptest.BootstrapWorkspaceSession(t, e, telegramWorkspaceName, telegramAdminEmail, "Admin")
	c := &telegramEnv{AppEnv: e, vault: vault, api: &fakeTelegramAPI{}, inserter: inserter, log: quiet}
	c.resolveActors(t)
	return c
}

type waNumber struct {
	id, phoneID, waba, secret, verify, team string
}

func (c *telegramEnv) connectNumber(t *testing.T, phoneID string) waNumber {
	t.Helper()
	return c.connectNumberFor(t, phoneID, c.readerTeam(t).String())
}

// connectNumberFor connects the number with an existing reader team, as a
// reconnect of the same number must.
func (c *telegramEnv) connectNumberFor(t *testing.T, phoneID, team string) waNumber {
	t.Helper()
	n := waNumber{phoneID: phoneID, waba: "WABA-" + phoneID, secret: "secret-" + phoneID, team: team}
	var made struct {
		Connection struct {
			ID, MailVisibility, SeatID string
		}
		VerifyToken string
	}
	status := c.Call(t, http.MethodPost, "/v1/channel-connections/whatsapp", integration.AnyMap{
		"wabaId": n.waba, "phoneNumberId": phoneID, "displayPhoneNumber": "+49 30 " + phoneID,
		"accessToken": "EAAG-" + phoneID, "appSecret": n.secret, "readerTeamId": n.team,
	}, nil, &made)
	if status != http.StatusCreated {
		t.Fatalf("connecting number %s → %d, want 201", phoneID, status)
	}
	n.id, n.verify = made.Connection.ID, made.VerifyToken
	if made.Connection.MailVisibility != "held" || made.Connection.SeatID == "" {
		t.Fatalf("a new number is %+v, want a held seat", made.Connection)
	}
	return n
}

func (n waNumber) sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(n.secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (c *telegramEnv) deliverWA(t *testing.T, n waNumber, body string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.TS.URL+"/v1/channel-events/whatsapp", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", n.sign([]byte(body)))
	resp, err := c.Client.Do(req) //nolint:bodyclose // closed by apptest.CloseBody below; bodyclose only recognises a Close in the same package
	if err != nil {
		t.Fatalf("delivering to the webhook: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	return resp.StatusCode
}

func waChange(field, phoneID, contacts, arrayName, items string) string {
	return fmt.Sprintf(`{"object":"whatsapp_business_account","entry":[{"id":"WABA-%s","changes":[{"field":%q,"value":{`+
		`"metadata":{"display_phone_number":"15550001111","phone_number_id":%q},"contacts":[%s],%q:[%s]}}]}]}`,
		phoneID, field, phoneID, contacts, arrayName, items)
}

const waAda = `{"profile":{"name":"Ada"},"wa_id":"491701234567"}`

func waInbound(phoneID, wamid, text string) string {
	return waChange("messages", phoneID, waAda, "messages",
		fmt.Sprintf(`{"from":"491701234567","id":%q,"timestamp":"1764000300","type":"text","text":{"body":%q}}`, wamid, text))
}

func waEcho(phoneID, wamid, text string) string {
	return waChange("smb_message_echoes", phoneID, waAda, "message_echoes",
		fmt.Sprintf(`{"from":"15550001111","to":"491701234567","id":%q,"timestamp":"1764000400","type":"text","text":{"body":%q}}`, wamid, text))
}

func waStatus(phoneID, wamid string) string {
	return waChange("messages", phoneID, "", "statuses",
		fmt.Sprintf(`{"id":%q,"status":"delivered","timestamp":"1764000500","recipient_id":"491701234567"}`, wamid))
}

func (c *telegramEnv) waActivities(t *testing.T) int {
	return c.count(t, `SELECT count(*) FROM activity WHERE source_system = 'whatsapp'`)
}

func (c *telegramEnv) waFiled(t *testing.T, wamid string) (filedMessage, bool) {
	t.Helper()
	var f filedMessage
	var found bool
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT a.id::text, a.direction, a.audience, a.thread_key, coalesce(l.contact_id::text, '')
			  FROM activity a LEFT JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'contact'
			 WHERE a.source_system = 'whatsapp' AND a.source_id LIKE '%:' || $1`, wamid)
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

func awaitWA(t *testing.T, sub <-chan *river.Event) {
	t.Helper()
	awaitJobKind(t, sub, compose.WhatsAppIngestArgs{}.Kind())
}

func TestAWhatsAppCustomerMessageAndTheBusinessAppEchoFileInOneThreadOnOneContact(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7001")

	if status := c.deliverWA(t, n, waInbound(n.phoneID, "wamid.IN1", "Is it in stock?")); status != http.StatusOK {
		t.Fatalf("customer message → %d", status)
	}
	awaitWA(t, sub)
	if status := c.deliverWA(t, n, waEcho(n.phoneID, "wamid.OUT1", "Yes, shipping Monday.")); status != http.StatusOK {
		t.Fatalf("echo → %d", status)
	}
	awaitWA(t, sub)

	in, ok1 := c.waFiled(t, "wamid.IN1")
	out, ok2 := c.waFiled(t, "wamid.OUT1")
	if !ok1 || !ok2 {
		t.Fatalf("filed customer message %v, echo %v, want both", ok1, ok2)
	}
	if in.direction != "inbound" || out.direction != "outbound" {
		t.Errorf("directions = %q and %q, want inbound and outbound", in.direction, out.direction)
	}
	if in.thread != "whatsapp:biz:7001:491701234567" || out.thread != in.thread {
		t.Errorf("threads = %q and %q, want one thread whatsapp:biz:7001:491701234567", in.thread, out.thread)
	}
	if in.contact == "" || out.contact != in.contact {
		t.Errorf("contacts = %q and %q, want one customer contact", in.contact, out.contact)
	}
	assertWhatsAppCustomerPhoneAndUpgrade(t, c, in.contact)
	if got := c.count(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id IN (@inbound, @outbound) AND contact_id = @contact AND channel_user_id = '491701234567'`,
		pgx.NamedArgs{"inbound": in.id, "outbound": out.id, "contact": in.contact}); got != 2 {
		t.Errorf("%d customer participants name the filed contact, want 2", got)
	}
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity_participant SET contact_id = NULL
			WHERE activity_id IN (@inbound, @outbound) AND channel_user_id = '491701234567'`,
			pgx.NamedArgs{"inbound": in.id, "outbound": out.id})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	store := contacts.NewStore(c.DB())
	contact := ids.From[ids.ContactKind](ids.MustParse(in.contact))
	owed, err := store.ContactsOwedACohortRepair(c.adminCtx(t), 100)
	if err != nil || !slices.Contains(owed, contact) {
		t.Fatalf("legacy channel contact is absent from the repair backlog: %v", err)
	}
	worker := compose.NewLinkReconcileWorkspaceWorkerForTest(c.Pool)
	for range 2 {
		if err := worker.ReconcileWorkspaceForTest(context.Background(), c.workspaceID(t)); err != nil {
			t.Fatal(err)
		}
	}
	if got := c.count(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id IN (@inbound, @outbound) AND contact_id = @contact AND channel_user_id = '491701234567'`,
		pgx.NamedArgs{"inbound": in.id, "outbound": out.id, "contact": in.contact}); got != 2 {
		t.Errorf("%d legacy customer participants were repaired, want 2", got)
	}
	owed, err = store.ContactsOwedACohortRepair(c.adminCtx(t), 100)
	if err != nil || slices.Contains(owed, contact) {
		t.Errorf("repaired channel contact stays in the backlog: %v", err)
	}

	c.deliverWA(t, n, waInbound(n.phoneID, "wamid.IN1", "Is it in stock?"))
	if a, r, j := c.waActivities(t), c.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'whatsapp'`),
		c.count(t, `SELECT count(*) FROM river_job WHERE kind = 'whatsapp_ingest'`); a != 2 || r != 2 || j != 2 {
		t.Errorf("after a redelivery %d activities, %d raw rows and %d jobs, want 2 of each", a, r, j)
	}
	var customer crmcontracts.Contact
	if status := c.Call(t, "GET", "/v1/contacts/"+in.contact, nil, nil, &customer); status != http.StatusOK {
		t.Fatalf("read customer: %d", status)
	}
	if customer.Version == nil {
		t.Fatal("customer read has no version")
	}
	if customer.Phones == nil || len(*customer.Phones) != 1 || (*customer.Phones)[0].Phone != "+491701234567" {
		t.Fatal("customer read does not show the captured phone")
	}
	var update integration.AnyMap
	if status := c.Call(t, "PATCH", "/v1/contacts/"+in.contact, integration.AnyMap{"phones": []integration.AnyMap{}}, map[string]string{"If-Match": fmt.Sprintf("%d", *customer.Version)}, &update); status != http.StatusOK {
		t.Fatalf("remove customer phone: %d, %v", status, update)
	}
	c.deliverWA(t, n, waInbound(n.phoneID, "wamid.IN2", "Another question"))
	awaitWA(t, sub)
	if filed, ok := c.waFiled(t, "wamid.IN2"); !ok || filed.contact != in.contact {
		t.Fatal("the message after removing the phone was not filed on its customer")
	}
	if count := c.count(t, `SELECT count(*) FROM contact_phone WHERE contact_id=@contact AND archived_at IS NULL`, pgx.NamedArgs{"contact": in.contact}); count != 0 {
		t.Fatal("a new message restored a phone removed from the contact")
	}
	assertRestrictedWhatsAppPhoneUpgrade(t, c, contact)
}

func TestAWhatsAppStatusNeverCreatesAnEntry(t *testing.T) {
	c := setupWhatsApp(t)
	runner, _ := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7002")

	if status := c.deliverWA(t, n, waStatus(n.phoneID, "wamid.S1")); status != http.StatusOK {
		t.Fatalf("status delivery → %d", status)
	}
	if a, r, j := c.waActivities(t), c.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'whatsapp'`),
		c.count(t, `SELECT count(*) FROM river_job WHERE kind = 'whatsapp_ingest'`); a != 0 || r != 0 || j != 0 {
		t.Errorf("a status left %d activities, %d raw rows and %d jobs, want none", a, r, j)
	}
}

func TestABSUIDOnlyCustomerFilesUnderTheBusinessAccountScope(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7003")
	body := waChange("messages", n.phoneID, `{"profile":{"name":"Bo"},"user_id":"US.1349"}`, "messages",
		`{"from_user_id":"US.1349","id":"wamid.B1","timestamp":"1764000300","type":"text","text":{"body":"hello"}}`)

	if status := c.deliverWA(t, n, body); status != http.StatusOK {
		t.Fatalf("delivery → %d", status)
	}
	awaitWA(t, sub)
	if got := c.count(t, `SELECT count(*) FROM contact_channel_identity WHERE provider = 'whatsapp' AND channel_user_id = 'US.1349' AND account_scope = $1`, n.waba); got != 1 {
		t.Errorf("%d identities for the BSUID under the account scope %s, want 1", got, n.waba)
	}
}

func TestADisconnectBetweenStoringAndFilingLeavesNothing(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	n := c.connectNumber(t, "7004")

	if status := c.deliverWA(t, n, waInbound(n.phoneID, "wamid.D1", "stored first")); status != http.StatusOK {
		t.Fatalf("delivery → %d", status)
	}
	if got := c.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'whatsapp'`); got != 1 {
		t.Fatalf("%d raw rows stored before the disconnect, want 1", got)
	}
	if status := c.Call(t, http.MethodDelete, "/v1/channel-connections/whatsapp/"+n.id, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("disconnect → %d, want 204", status)
	}
	startTelegramWorker(t, runner)
	awaitWA(t, sub)
	if a, r := c.waActivities(t), c.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'whatsapp'`); a != 0 || r != 0 {
		t.Errorf("a disconnected number left %d activities and %d raw rows, want none", a, r)
	}
	if got := c.count(t, `SELECT count(*) FROM capture_connection WHERE provider = 'whatsapp' AND status = 'disconnected'`); got != 1 {
		t.Errorf("%d disconnected seats, want the number's seat ended", got)
	}
}

func TestAHeldNumberKeepsMessagesFromColleaguesAndSharingOpensTheNextOne(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7005")
	path := "/v1/channel-connections/whatsapp/" + n.id + "/visibility"

	c.deliverWA(t, n, waInbound(n.phoneID, "wamid.H1", "held"))
	awaitWA(t, sub)
	if f, _ := c.waFiled(t, "wamid.H1"); f.audience != "participants" {
		t.Fatalf("a message on a held number is %q, want participants", f.audience)
	}

	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "shared", "confirm": true}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("shared without the workspace opt-in → %d, want 422", status)
	}
	c.allowSharedPosture(t)
	var changed struct{ MailVisibility string }
	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "shared", "confirm": true}, nil, &changed); status != http.StatusOK || changed.MailVisibility != "shared" {
		t.Fatalf("sharing → %d %q, want 200 shared", status, changed.MailVisibility)
	}

	c.deliverWA(t, n, waInbound(n.phoneID, "wamid.H2", "shared"))
	awaitWA(t, sub)
	if f, _ := c.waFiled(t, "wamid.H2"); f.audience != "workspace" {
		t.Errorf("the next message is %q, want workspace", f.audience)
	}
	if f, _ := c.waFiled(t, "wamid.H1"); f.audience != "participants" {
		t.Errorf("sharing published the earlier held message: %q", f.audience)
	}
}

func TestABusinessThreadOnWhatsAppCannotBeRepliedTo(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7006")
	c.deliverWA(t, n, waInbound(n.phoneID, "wamid.R1", "question"))
	awaitWA(t, sub)
	filed, _ := c.waFiled(t, "wamid.R1")
	c.grantConsent(t, filed.contact, "transactional")

	status := c.Call(t, http.MethodPost, "/v1/activities/"+filed.id+"/send-message", integration.AnyMap{
		"body": "reply from the CRM", "consent_purpose": "transactional",
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a reply into a WhatsApp business thread → %d, want 422", status)
	}
	if got := c.count(t, `SELECT count(*) FROM activity WHERE direction = 'outbound' AND thread_key = $1`, filed.thread); got != 0 {
		t.Errorf("%d outbound activities staged on the thread, want none", got)
	}
}

func waReaction(phoneID, wamid, onto string) string {
	return waChange("messages", phoneID, waAda, "messages",
		fmt.Sprintf(`{"from":"491701234567","id":%q,"timestamp":"1764000350","type":"reaction","reaction":{"message_id":%q,"emoji":"+"}}`, wamid, onto))
}

func waRevoke(phoneID, wamid, original string) string {
	return waChange("messages", phoneID, waAda, "messages",
		fmt.Sprintf(`{"from":"491701234567","id":%q,"timestamp":"1764000360","type":"revoke","revoke":{"original_message_id":%q}}`, wamid, original))
}

func (c *telegramEnv) waRaw(t *testing.T) int {
	return c.count(t, `SELECT count(*) FROM raw_capture WHERE source_system = 'whatsapp'`)
}

func (c *telegramEnv) eraserCtx(t *testing.T) context.Context {
	t.Helper()
	ctx := c.adminCtx(t)
	actor, _ := principal.Actor(ctx)
	actor.Permissions = integration.AdminPerms
	return originalstore.Bind(principal.WithActor(ctx, actor), c.DB(), nil)
}

func TestErasingAWhatsAppContactDestroysTheRawRowOfItsMessageAndOfItsEdit(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7101")
	c.deliverWA(t, n, waInbound(n.phoneID, "wamid.P1", "filed message"))
	awaitWA(t, sub)
	c.deliverWA(t, n, waEdit(n.phoneID, "wamid.P2", "wamid.P1", "edited message", 1764000360))
	awaitWA(t, sub)
	filed, _ := c.waFiled(t, "wamid.P1")
	if got := c.waRaw(t); got != 1 {
		t.Fatalf("%d raw rows before the erasure, want the edit's row, which replaced the original", got)
	}
	contact, err := ids.Parse(filed.contact)
	if err != nil {
		t.Fatal(err)
	}

	pkg, err := privacy.AssembleSAR(c.eraserCtx(t), c.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.RawCapture) != 1 {
		t.Errorf("the access export lists %d raw originals, want the edit's", len(pkg.RawCapture))
	}
	if err := privacy.NewEraser(c.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).EraseContact(c.eraserCtx(t), contact, "test"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
	if got := c.waRaw(t); got != 0 {
		t.Errorf("%d WhatsApp raw originals survive the erasure, want none", got)
	}
}

func TestACustomerErasedBetweenStoringAndFilingLeavesNoOriginal(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	n := c.connectNumber(t, "7102")
	if status := c.deliverWA(t, n, waInbound(n.phoneID, "wamid.E1", "stored first")); status != http.StatusOK {
		t.Fatalf("delivery → %d", status)
	}
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO erasure_suppression (kind, value_hash) VALUES ('channel_identity', $1)`,
			storekit.ChannelIdentityHash("whatsapp", "491701234567"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	startTelegramWorker(t, runner)
	awaitWA(t, sub)
	if a, r := c.waActivities(t), c.waRaw(t); a != 0 || r != 0 {
		t.Errorf("an erased customer left %d activities and %d raw rows, want none", a, r)
	}
}

func TestAMessageNamingAPhoneAndABSUIDBindsBothAndErasureSuppressesBoth(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7103")
	both := waChange("messages", n.phoneID, `{"profile":{"name":"Cy"},"wa_id":"491709990001","user_id":"US.77"}`, "messages",
		`{"from":"491709990001","from_user_id":"US.77","id":"wamid.X1","timestamp":"1764000300","type":"text","text":{"body":"hi"}}`)
	c.deliverWA(t, n, both)
	awaitWA(t, sub)
	filed, _ := c.waFiled(t, "wamid.X1")
	if got := c.count(t, `SELECT count(*) FROM contact_channel_identity WHERE contact_id = $1::uuid AND provider = 'whatsapp'`, filed.contact); got != 2 {
		t.Fatalf("the contact holds %d WhatsApp identities, want its phone and its BSUID", got)
	}
	contact, _ := ids.Parse(filed.contact)
	if err := privacy.NewEraser(c.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).EraseContact(c.eraserCtx(t), contact, "test"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}

	bsuidOnly := waChange("messages", n.phoneID, `{"profile":{"name":"Cy"},"user_id":"US.77"}`, "messages",
		`{"from_user_id":"US.77","id":"wamid.X2","timestamp":"1764000400","type":"text","text":{"body":"back again"}}`)
	c.deliverWA(t, n, bsuidOnly)
	if _, found := c.waFiled(t, "wamid.X2"); found || c.waRaw(t) != 0 {
		t.Errorf("a later BSUID-only delivery was filed (%v) or left %d raw rows, want nothing", found, c.waRaw(t))
	}
}

func waEdit(phoneID, wamid, original, text string, at int) string {
	return waChange("messages", phoneID, waAda, "messages",
		fmt.Sprintf(`{"from":"491701234567","id":%q,"timestamp":"%d","type":"edit","edit":{"original_message_id":%q,"text":{"body":%q}}}`, wamid, at, original, text))
}

func (c *telegramEnv) waBody(t *testing.T, wamid string) string {
	t.Helper()
	var body string
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(body, '') FROM activity WHERE source_system = 'whatsapp' AND source_id LIKE '%:' || $1`, wamid).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the filed body: %v", err)
	}
	return body
}

func TestAReactionLeavesNoRowAndNoEntry(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7104")
	c.deliverWA(t, n, waReaction(n.phoneID, "wamid.C1", "wamid.GONE"))
	awaitWA(t, sub)
	if a, r := c.waActivities(t), c.waRaw(t); a != 0 || r != 0 {
		t.Errorf("%d activities and %d raw rows after a reaction, want none", a, r)
	}
}

func TestAWhatsAppEditChangesTheFiledTextAndARevokeOnACompanyNumberKeepsIt(t *testing.T) {
	c := setupWhatsApp(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	n := c.connectNumber(t, "7105")
	c.deliverWA(t, n, waInbound(n.phoneID, "wamid.A1", "first words"))
	awaitWA(t, sub)

	c.deliverWA(t, n, waEdit(n.phoneID, "wamid.A2", "wamid.A1", "corrected words", 1764000360))
	awaitWA(t, sub)
	if got := c.waBody(t, "wamid.A1"); got != "corrected words" {
		t.Fatalf("body %q after the edit, want the edited text", got)
	}
	if a, r := c.waActivities(t), c.waRaw(t); a != 1 || r != 1 {
		t.Fatalf("%d activities and %d raw rows after the edit, want one message holding one original", a, r)
	}

	c.deliverWA(t, n, waEdit(n.phoneID, "wamid.A3", "wamid.A1", "older words", 1764000330))
	awaitWA(t, sub)
	if got := c.waBody(t, "wamid.A1"); got != "corrected words" {
		t.Fatalf("body %q after an older edit, want the newest text", got)
	}
	if r := c.waRaw(t); r != 1 {
		t.Fatalf("%d raw rows after an older edit, want the edit that changed nothing deleted", r)
	}

	c.deliverWA(t, n, waRevoke(n.phoneID, "wamid.A4", "wamid.A1"))
	awaitWA(t, sub)
	if got := c.waBody(t, "wamid.A1"); got != "corrected words" {
		t.Fatalf("body %q after a revoke on a company number, want the team's copy kept", got)
	}
	if a, r := c.waActivities(t), c.waRaw(t); a != 1 || r != 1 {
		t.Fatalf("%d activities and %d raw rows after the revoke, want the message and its one original", a, r)
	}
}

func assertWhatsAppCustomerPhoneAndUpgrade(t *testing.T, c *telegramEnv, contact string) {
	t.Helper()
	phoneCount := func() int {
		return c.count(t, `SELECT count(*) FROM contact_phone WHERE contact_id=@contact AND phone='+491701234567' AND archived_at IS NULL`, pgx.NamedArgs{"contact": contact})
	}
	if phoneCount() != 1 {
		t.Fatal("captured customer number is absent from the phone fields")
	}
	if count := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type='contact' AND entity_id=@contact AND jsonb_typeof("after"->'phones')='array'`, pgx.NamedArgs{"contact": contact}); count == 0 {
		t.Fatal("phone change has no restorable set audit image")
	}
	if count := c.count(t, `SELECT count(*) FROM field_provenance WHERE object_type='contact' AND object_id=@contact AND field_name='phone' AND source='whatsapp' AND evidence_ref IS NOT NULL`, pgx.NamedArgs{"contact": contact}); count == 0 {
		t.Fatal("captured phone has no provider evidence")
	}

	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `DELETE FROM contact_phone WHERE contact_id=@contact`, pgx.NamedArgs{"contact": contact})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	task := contacts.ChannelPhonesDataTask()
	task.Steps[0].Limit = 1
	task.Steps[0].MinLimit = 1
	now := clocktest.Now(t)
	ctx := principal.SystemActing(c.adminCtx(t), "system:channel-phone-test")
	rollbackTask := task
	rollbackTask.ID += "_rollback"
	rollbackTask.Steps = slices.Clone(task.Steps)
	batch := task.Steps[0].Batch
	injected := false
	rollbackTask.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := batch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		if out.Changed > 0 {
			injected = true
			return out, errors.New("injected transaction failure")
		}
		return out, nil
	}
	rollback, err := datatask.New(c.Pool, []datatask.Task{rollbackTask}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := rollback.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range c.count(t, `SELECT count(*) FROM contact_channel_identity`) + 1 {
		if _, err := rollback.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		if injected {
			break
		}
	}
	if !injected {
		t.Fatal("the rollback probe never reached a phone write")
	}
	if phoneCount() != 0 {
		t.Fatal("failed correction committed a phone outside its progress transaction")
	}
	engine, err := datatask.New(c.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	progress, err := engine.List(ctx)
	if err != nil || len(progress) != 1 || progress[0].Status != "done" {
		t.Fatalf("phone upgrade did not finish: %+v, %v", progress, err)
	}
	if phoneCount() != 1 {
		t.Fatal("upgrade did not restore the customer number exactly once")
	}
}

func assertRestrictedWhatsAppPhoneUpgrade(t *testing.T, c *telegramEnv, contact ids.ContactID) {
	t.Helper()
	privacyCtx := c.eraserCtx(t)
	if err := consent.NewStore(c.DB()).Suppress(privacyCtx, consent.SuppressInput{ContactID: contact, Kind: "processing_restriction", Reason: "Disputed contact data."}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(privacyCtx, "system:channel-phone-test")
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM contact_phone WHERE contact_id=@contact`, pgx.NamedArgs{"contact": contact}); err != nil {
			return err
		}
		out, err := contacts.ChannelPhonesDataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100})
		if err != nil {
			return err
		}
		if out.Scanned == 0 || out.Changed != 0 {
			t.Fatalf("restricted phone upgrade: scanned=%d changed=%d", out.Scanned, out.Changed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count := c.count(t, `SELECT count(*) FROM contact_phone WHERE contact_id=@contact`, pgx.NamedArgs{"contact": contact}); count != 0 {
		t.Fatal("the upgrade derived a phone while processing was restricted")
	}
}
