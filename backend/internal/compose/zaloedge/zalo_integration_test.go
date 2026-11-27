// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package zaloedge_test

// The Zalo connection and webhook edge over a real Postgres. An admin connects
// Official Accounts and Zalo's events are checked against the sealed keys. Every
// accepted message is one raw_capture row and one ingest job.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAnAdminConnectsAnOAAndARepCannot(t *testing.T) {
	e := setup(t)
	req := capture.ConnectZaloOARequest{OAID: "9990001", AppID: appID, DisplayName: "Shop", AppSecret: "s1", OASecretKey: "k1", AccessToken: "a1", RefreshToken: "r1", ReaderTeam: e.team}

	if _, err := e.store.ConnectZaloOA(e.rep, req); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep connecting got %v, want a permission denial", err)
	}
	if n := e.count(t, `SELECT count(*) FROM channel_connection WHERE provider = 'zalo_oa'`); n != 0 {
		t.Fatalf("%d rows after a refused connect, want none", n)
	}
	conn, err := e.store.ConnectZaloOA(e.admin, req)
	if err != nil {
		t.Fatal(err)
	}
	var provider, account, channel, status string
	var approved bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT provider, account_id, channel_id, status, approved_at IS NOT NULL FROM channel_connection WHERE id = $1`, conn.ID).
		Scan(&provider, &account, &channel, &status, &approved); err != nil {
		t.Fatal(err)
	}
	if provider != "zalo_oa" || account != "9990001" || channel != appID || status != "connected" || !approved {
		t.Errorf("row = %s/%s/%s/%s approved=%v, want a connected, approved zalo_oa row for OA 9990001", provider, account, channel, status, approved)
	}
	if _, err := e.store.ConnectZaloOA(e.admin, req); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("connecting the same OA twice got %v, want a conflict", err)
	}
	if n := e.vault.(interface{ Count() int }).Count(); n != 2 {
		t.Errorf("the vault holds %d secrets, want 2: the refused second connect must leave none", n)
	}
}

func TestSecretsAreSealedAndNeverReturnedOrAudited(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990002", "oa-secret-key-2")
	list, err := e.store.ListZaloOAConnections(e.admin)
	if err != nil || len(list) != 1 || list[0].OAID != "9990002" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if _, err := e.store.ListZaloOAConnections(e.rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep listing got %v, want a permission denial", err)
	}
	var audit, row string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(string_agg(before::text || after::text, ' '), '') FROM audit_log WHERE entity_type = 'channel_connection'`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(context.Background(), `SELECT row_to_json(c)::text FROM channel_connection c WHERE id = $1`, a.conn.ID).Scan(&row); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"oa-secret-key-2", "app-secret-9990002", "access-9990002", "refresh-9990002"} {
		if strings.Contains(audit, secret) || strings.Contains(row, secret) || strings.Contains(fmt.Sprintf("%+v", list), secret) {
			t.Errorf("secret %q reachable outside the vault", secret)
		}
	}
}

func TestABadSignatureStoresNothing(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990003", "k3")
	body := inbound(a.oaID, "5550001", "msg-1", "hello")
	for name, signature := range map[string]string{
		"none": "", "wrong secret": sign("other", body),
		"tampered": sign(a.secret, append([]byte(nil), body[1:]...)), "not a mac": "sha256=abcd",
	} {
		if rec := e.post(body, signature); rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, rec.Code)
		}
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("a refused delivery left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestASignedMessageStoresOneRowAndOneJobAndARetryAddsNone(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990004", "k4")
	body := inbound(a.oaID, "5550001", "msg-1", "one")

	rec := e.post(body, sign(a.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":1`) {
		t.Fatalf("status %d body %s, want 200 with one stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 1 || e.ingestJobs(t) != 1 {
		t.Fatalf("%d raw rows and %d jobs, want 1 and 1", e.rawRows(t), e.ingestJobs(t))
	}
	var source string
	if err := e.owner.QueryRow(context.Background(), `SELECT source_id FROM raw_capture WHERE source_system = 'zalo_oa'`).Scan(&source); err != nil || source != "9990004|msg-1" {
		t.Errorf("source id = %q, %v, want OA id and msg_id", source, err)
	}
	if again := e.post(body, sign(a.secret, body)); again.Code != http.StatusOK {
		t.Fatalf("retry status %d, want 200", again.Code)
	}
	if e.rawRows(t) != 1 || e.ingestJobs(t) != 1 {
		t.Errorf("after a retry %d raw rows and %d jobs, want 1 and 1", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestNonMessageEventsAreAcknowledgedAndStoreNothing(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990005", "k5")
	for name, event := range map[string]string{
		"follow":   `"event_name":"follow","follower":{"id":"5550001"},"oa_id":"9990005"`,
		"unfollow": `"event_name":"unfollow","follower":{"id":"5550001"},"oa_id":"9990005"`,
		"seen":     `"event_name":"user_seen_message","sender":{"id":"5550001"},"recipient":{"id":"9990005"},"message":{"msg_ids":["m1"]}`,
		"reaction": `"event_name":"user_reacted_message","sender":{"id":"5550001"},"recipient":{"id":"9990005"},"message":{"msg_id":"m1"}`,
	} {
		body := []byte(fmt.Sprintf(`{"app_id":%q,"timestamp":%q,%s}`, appID, timestamp, event))
		rec := e.post(body, sign(a.secret, body))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) {
			t.Errorf("%s: status %d body %s, want 200 with none stored", name, rec.Code, rec.Body.String())
		}
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("non-message events left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestAnUnknownOAStoresNothing(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990006", "k6")
	body := inbound("someone-elses-oa", "5550001", "msg-x", "stray")
	rec := e.post(body, sign(a.secret, body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Errorf("status %d body %s, want 200 with none stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Errorf("an unknown OA left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
}

func TestASecretCoversOnlyItsOwnOA(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990007", "k7")
	b := e.connect(t, "9990008", "k8")
	forB := inbound(b.oaID, "5550001", "msg-b", "for b")
	if rec := e.post(forB, sign(a.secret, forB)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Fatalf("an event for B signed with A's key: %d %s, want 200 with none stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 0 {
		t.Fatalf("A's key stored %d rows for B", e.rawRows(t))
	}
	if rec := e.post(forB, sign(b.secret, forB)); !strings.Contains(rec.Body.String(), `"stored":1`) {
		t.Errorf("B's own key: %s, want it stored", rec.Body.String())
	}
}

func TestAnEventNamingAnotherAppIsNotStored(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990009", "k9")
	body := []byte(strings.Replace(string(inbound(a.oaID, "5550001", "msg-app", "hi")), appID, "9999999999", 1))
	// The MAC covers the body's own app id, so only the connection's app binding refuses it.
	if rec := e.post(body, signFor("9999999999", a.secret, body)); !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Errorf("an event of another app: %s, want none stored", rec.Body.String())
	}
}

func TestASuppressedCustomerIsNotStoredButTheSameIDUnderAnotherOAIs(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990010", "k10")
	b := e.connect(t, "9990011", "k11")
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO erasure_suppression (kind, value_hash) VALUES ('channel_identity', $1)`,
		storekit.ChannelIdentityHash("zalo_oa", storekit.ChannelSuppressionAccount(a.oaID, "5550001"))); err != nil {
		t.Fatal(err)
	}
	erased := inbound(a.oaID, "5550001", "msg-e", "from an erased customer")
	if rec := e.post(erased, sign(a.secret, erased)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":0`) {
		t.Errorf("erased customer: %d %s, want none stored", rec.Code, rec.Body.String())
	}
	if e.rawRows(t) != 0 || e.ingestJobs(t) != 0 {
		t.Fatalf("an erased customer left %d raw rows and %d jobs", e.rawRows(t), e.ingestJobs(t))
	}
	neighbour := inbound(a.oaID, "5550002", "msg-n", "another customer of the same OA")
	if rec := e.post(neighbour, sign(a.secret, neighbour)); !strings.Contains(rec.Body.String(), `"stored":1`) {
		t.Errorf("another customer under the same OA: %s, want it stored", rec.Body.String())
	}
	other := inbound(b.oaID, "5550001", "msg-o", "same id, other OA")
	if rec := e.post(other, sign(b.secret, other)); !strings.Contains(rec.Body.String(), `"stored":1`) {
		t.Errorf("the same user id under another OA: %s, want it stored", rec.Body.String())
	}
}

func TestDisconnectStopsAcceptanceAndDestroysTheSecrets(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9990012", "k12")
	body := inbound(a.oaID, "5550001", "msg-d1", "before")
	if rec := e.post(body, sign(a.secret, body)); rec.Code != http.StatusOK || e.rawRows(t) != 1 {
		t.Fatalf("status %d, %d rows before the disconnect", rec.Code, e.rawRows(t))
	}
	if err := e.store.DisconnectZaloOA(e.rep, a.conn.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep disconnecting got %v, want a permission denial", err)
	}
	if err := e.store.DisconnectZaloOA(e.admin, a.conn.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.store.DisconnectZaloOA(e.admin, a.conn.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("disconnecting twice got %v, want not found", err)
	}
	after := inbound(a.oaID, "5550001", "msg-d2", "after")
	if rec := e.post(after, sign(a.secret, after)); rec.Code != http.StatusForbidden {
		t.Errorf("an event after the disconnect got %d, want 403", rec.Code)
	}
	if e.rawRows(t) != 1 {
		t.Errorf("%d raw rows after the disconnect, want the one stored before it", e.rawRows(t))
	}
	if left := e.vault.(interface{ Count() int }).Count(); left != 0 {
		t.Errorf("the vault still holds %d secrets, want none", left)
	}
	if _, err := e.store.ConnectZaloOA(e.admin, capture.ConnectZaloOARequest{
		OAID: a.oaID, AppID: appID, DisplayName: "d", AppSecret: "s", OASecretKey: "k", AccessToken: "t", RefreshToken: "r", ReaderTeam: e.team,
	}); err != nil {
		t.Errorf("connecting the OA again after a disconnect: %v", err)
	}
}
