// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A send whose read-back of its provider-stamped identity failed. The message
// went out once and must not go out again. Its captured echo carries the
// identity, and the send and the echo meet on one timeline row in either order.
//
// Everything is the production object but Google, including the delivery
// store and the capture sink compose builds.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// gmailSentID is the id the stubbed Gmail gives every message it accepts. The
// captured echo carries it, which is how the echo finds the send.
const gmailSentID = "gmail-msg-1"

// refusingReadBackGmail accepts every send, counts it, and answers the
// read-back of the sent message with a refusal.
type refusingReadBackGmail struct {
	sends atomic.Int32
	raw   atomic.Value
}

func (g *refusingReadBackGmail) serve(t *testing.T, status int, reason string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/messages/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		WriteJSON(w, map[string]any{"error": map[string]any{"errors": []map[string]any{{"reason": reason}}}})
	})
	mux.HandleFunc("/messages/send", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Raw string `json:"raw"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the send body: %v", err)
			return
		}
		g.sends.Add(1)
		g.raw.Store(body.Raw)
		WriteJSON(w, map[string]any{"id": gmailSentID, "threadId": "gmail-thread-1"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, map[string]any{"access_token": "access-tok", "refresh_token": "refresh-tok", "expires_in": 3599})
	})
	mux.HandleFunc("/profile", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, map[string]any{"emailAddress": sendingMailbox, "historyId": "1001"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// transmitted is what the stub was asked to send, as the provider would store
// it under the identity it minted.
func (g *refusingReadBackGmail) transmitted(t *testing.T) []byte {
	t.Helper()
	raw, ok := g.raw.Load().(string)
	if !ok {
		t.Fatal("nothing was sent")
	}
	mime, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("the connector did not hand Gmail base64url: %v", err)
	}
	return storedCopy(t, mime, gmailStamped)
}

// recoveryEnv is one staged send from the rep's connected Gmail.
type recoveryEnv struct {
	*preflightEnv
	activity, delivery ids.UUID
	minted             string
}

func setupRecovery(t *testing.T) recoveryEnv {
	t.Helper()
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	// The address the grant proved, which is what lets capture record this
	// mailbox as having delivered the echo.
	p.inWorkspace(t, `UPDATE capture_connection SET account_label = $2 WHERE user_id = $1`, p.user, sendingMailbox)
	p.stakeADeal(t)
	activity := p.sendExpectingAcceptance(t, "transactional", "Re: Inbound question", "As discussed.")
	delivery, minted := p.deliveryFor(t, activity)
	return recoveryEnv{preflightEnv: p, activity: activity, delivery: delivery, minted: minted}
}

// sendRefusingReadBack transmits the staged message once through a Gmail that
// refuses the read-back with status, and insists the send itself succeeded.
func (r recoveryEnv) sendRefusingReadBack(t *testing.T, status int, reason string) *refusingReadBackGmail {
	t.Helper()
	gmail := &refusingReadBackGmail{}
	if outcome, _ := r.dispatchThrough(t, r.delivery, gmail.serve(t, status, reason), nil); outcome != comms.OutcomeSent {
		t.Fatalf("dispatch outcome = %q, want sent: a failed read-back is still a sent message", outcome)
	}
	return gmail
}

// captureOwnEcho drives the composed sink with the provider's copy of the
// message, filed in the rep's own mailbox under the provider's message id.
func (r recoveryEnv) captureOwnEcho(ctx context.Context, t *testing.T, stored []byte) {
	t.Helper()
	msg, err := mailmap.Parse(stored, sendingMailbox)
	if err != nil {
		t.Fatalf("the provider's stored copy does not parse:\n%s\n%v", stored, err)
	}
	rec := msg.AttestSentByOwner(true).ToRecord("gmail", stored)
	rec.ProviderMessageID = gmailSentID
	if _, err := compose.NewCaptureSink(r.Pool, compose.CaptureConfig{}).Upsert(ctx, rec); err != nil {
		t.Fatalf("capturing the provider's own copy: %v", err)
	}
}

func (r recoveryEnv) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := apptest.InWorkspace(r.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&out)
	}); err != nil {
		t.Fatalf("reading %q: %v", query, err)
	}
	return out
}

// convergedOnTheSend insists the timeline holds the message once, as the send's
// own row under the identity the provider stamped.
func (r recoveryEnv) convergedOnTheSend(t *testing.T) {
	t.Helper()
	if live := r.countActivities(t, `source_id = $1 AND archived_at IS NULL`, gmailStamped); live != 1 {
		t.Fatalf("%d live activities carry the stamped identity, want 1", live)
	}
	if id := r.activityOnTheStampedIdentity(t); id != r.activity {
		t.Fatalf("the stamped identity resolves to %s, want the send's own activity %s", id, r.activity)
	}
	if stale := r.countActivities(t, `source_id = $1`, r.minted); stale != 0 {
		t.Errorf("%d activities still carry the minted identity %q", stale, r.minted)
	}
	if got := r.scalar(t, `SELECT message_id FROM comms_outbound WHERE id = $1`, r.delivery); got != gmailStamped {
		t.Errorf("the delivery reads %q, want %q", got, gmailStamped)
	}
}

// A throttled read-back, then the echo in the normal order: the echo's capture
// re-keys the send and lands on it as a replay. The message was sent once, and
// the capture started no second activity.
func TestAThrottledReadBackThenItsEchoLeaveOneActivityAndOneSend(t *testing.T) {
	r := setupRecovery(t)
	gmail := r.sendRefusingReadBack(t, http.StatusTooManyRequests, "rateLimitExceeded")
	if r.countActivities(t, `source_id = $1`, r.minted) != 1 {
		t.Fatal("the send's row did not stay on its minted identity after the failed read-back; this case proves nothing")
	}

	captured := `SELECT count(*)::text FROM event_outbox WHERE envelope->>'type' = 'activity.captured'`
	before := r.scalar(t, captured)

	r.captureOwnEcho(r.ownMailbox(t), t, gmail.transmitted(t))

	r.convergedOnTheSend(t)
	if sends := gmail.sends.Load(); sends != 1 {
		t.Errorf("Gmail was asked to send %d times, want 1", sends)
	}
	if after := r.scalar(t, captured); after != before {
		t.Errorf("activity.captured events went from %s to %s: the echo started downstream work of its own", before, after)
	}
	if pending := r.scalar(t, `SELECT party_admission_pending::text FROM comms_outbound WHERE id = $1`, r.delivery); pending != "false" {
		t.Errorf("party_admission_pending = %s, want the send's admission settled", pending)
	}
}

// The echo lands before the receipt. The receipt finds it by the provider's
// message id and absorbs it, as after a read-back that answered.
func TestAnEchoCapturedBeforeAFailedReadBackIsAbsorbedByTheReceipt(t *testing.T) {
	r := setupRecovery(t)
	r.captureOwnEcho(r.ownMailbox(t), t, echoAlreadyCaptured("Re: Inbound question", gmailStamped))
	echo := r.activityOnTheStampedIdentity(t)
	if echo == r.activity {
		t.Fatal("the seeded echo IS the send's row; nothing would collide and this case would prove nothing")
	}

	gmail := r.sendRefusingReadBack(t, http.StatusServiceUnavailable, "backendError")

	r.convergedOnTheSend(t)
	if archived := r.scalar(t, `SELECT (archived_at IS NOT NULL)::text FROM activity WHERE id = $1`, echo); archived != "true" {
		t.Error("the absorbed echo is still on the timeline")
	}
	if audits := r.scalar(t, `SELECT count(*)::text FROM audit_log WHERE entity_id = $1 AND action = 'update'
		AND after->>'source_id' = $2`, r.activity, gmailStamped); audits != "1" {
		t.Errorf("%s audit rows for the re-key, want 1", audits)
	}
	if faults := r.scalar(t, `SELECT count(*)::text FROM system_log WHERE action = 'comms_identity_reconcile_failed'`); faults != "0" {
		t.Errorf("%s reconcile-fault breadcrumbs, want 0", faults)
	}
	if sends := gmail.sends.Load(); sends != 1 {
		t.Errorf("Gmail was asked to send %d times, want 1", sends)
	}
}

// A worker that died after the receipt committed and before the reconcile: the
// replayed job sends nothing, and the echo still finds the send.
func TestACrashAfterTheReceiptThenAReplayThenTheEchoSendsOnceAndConverges(t *testing.T) {
	r := setupRecovery(t)
	worker := compose.SendWorkerContext(context.Background(), r.workspaceID(t))
	// The durable state a crash after commitReceipt leaves: the receipt, and no
	// identity work at all.
	if err := compose.NewMailDeliveryStore(r.Pool, nil).RecordSent(worker, r.delivery,
		connector.SendReceipt{ProviderMessageID: gmailSentID}); err != nil {
		t.Fatalf("recording the receipt the crashed worker committed: %v", err)
	}

	replay := &refusingReadBackGmail{}
	outcome, _ := r.dispatchThrough(t, r.delivery, replay.serve(t, http.StatusTooManyRequests, "rateLimitExceeded"), nil)
	if sends := replay.sends.Load(); sends != 0 {
		t.Fatalf("the replayed job sent %d more times (outcome %q), want 0", sends, outcome)
	}
	r.captureOwnEcho(r.ownMailbox(t), t, echoAlreadyCaptured("Re: Inbound question", gmailStamped))

	r.convergedOnTheSend(t)
}

// A mailbox that refuses the read leaves an explicit outcome, and the send's
// row stays the one row.
func TestADeniedReadBackIsRecordedAndTheSendStaysOneRow(t *testing.T) {
	r := setupRecovery(t)
	gmail := r.sendRefusingReadBack(t, http.StatusForbidden, "insufficientPermissions")

	reason := r.scalar(t, `SELECT detail->>'reason' FROM system_log WHERE action = 'comms_identity_reconcile_failed'
		AND detail->>'delivery_id' = $1`, r.delivery.String())
	if !strings.Contains(reason, "read_denied") {
		t.Errorf("breadcrumb reason = %q, want the denied read named", reason)
	}
	if rows := r.countActivities(t, `source_id = $1 AND archived_at IS NULL`, r.minted); rows != 1 {
		t.Errorf("%d live rows on the staged identity, want the send's one", rows)
	}
	if sends := gmail.sends.Load(); sends != 1 {
		t.Errorf("Gmail was asked to send %d times, want 1", sends)
	}
}

// Another seat's mailbox can hold a message with the same Gmail id. Its echo
// is that seat's own mail and never re-keys this rep's send.
func TestAnEchoInAnotherMailboxLeavesTheSendAlone(t *testing.T) {
	r := setupRecovery(t)
	r.sendRefusingReadBack(t, http.StatusTooManyRequests, "rateLimitExceeded")
	r.DescribeCompany(t)
	colleague := uuidOf(t, createdID(t, r.AppEnv, "/v1/users", AnyMap{
		"email": "second-mailbox@recovery.test", "display_name": "Second Mailbox", "role": "rep",
	}))
	r.inWorkspace(t, `INSERT INTO capture_connection (provider, user_id, scopes, status, auth, provider_scopes)
		VALUES ('gmail', $1, '{}', 'connected', '{"refresh_token":"r","granted":[]}', $2)`,
		colleague, []string{apptest.GmailReadonlyScope})

	r.captureOwnEcho(seatMailboxCtx(t, r.workspaceID(t), colleague), t, echoAlreadyCaptured("Re: Inbound question", gmailStamped))

	if got := r.scalar(t, `SELECT message_id FROM comms_outbound WHERE id = $1`, r.delivery); got != r.minted {
		t.Errorf("the rep's delivery reads %q, want its staged %q", got, r.minted)
	}
	if rows := r.countActivities(t, `source_id = $1`, r.minted); rows != 1 {
		t.Errorf("%d rows on the rep's staged identity, want the send untouched", rows)
	}
}

// ownMailbox is the rep's own Gmail connector.
func (r recoveryEnv) ownMailbox(t *testing.T) context.Context {
	t.Helper()
	return seatMailboxCtx(t, r.workspaceID(t), uuidOf(t, r.user))
}

// seatMailboxCtx is a seat's own Gmail connector acting with that seat's
// authority, as the capture registry builds it for a sync (connectorActing).
func seatMailboxCtx(t *testing.T, ws, seat ids.UUID) context.Context {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: seat, OnBehalfOf: seat,
		Scopes:      principal.NewScopeSet(principal.ScopeRead),
		Permissions: AdminPerms,
	})
}
