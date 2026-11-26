// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package zaloedge_test

// The token renewal over a real Postgres and an httptest Zalo.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// failingPut is a vault whose writes can be switched off.
type failingPut struct {
	keyvault.Vault
	fail atomic.Bool
}

func (v *failingPut) Put(ctx context.Context, ws ids.WorkspaceID, secret []byte) (keyvault.Ref, error) {
	if v.fail.Load() {
		return "", errors.New("vault is down")
	}
	return v.Vault.Put(ctx, ws, secret)
}

type fakeZalo struct {
	srv     *httptest.Server
	calls   atomic.Int32
	mu      sync.Mutex
	refresh []string
	// answer writes the reply of one refresh; the default is a fresh pair.
	answer func(w http.ResponseWriter, n int32)
}

func newFakeZalo(t *testing.T) *fakeZalo {
	f := &fakeZalo{}
	f.answer = func(w http.ResponseWriter, n int32) {
		_, _ = fmt.Fprintf(w, `{"access_token":"access-%d","refresh_token":"refresh-%d","expires_in":"90000"}`, n, n)
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			f.mu.Lock()
			f.refresh = append(f.refresh, r.PostForm.Get("refresh_token"))
			f.mu.Unlock()
		}
		f.answer(w, f.calls.Add(1))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (e *env) renewer(f *fakeZalo, vault keyvault.Vault) *capture.ZaloRenewer {
	return &capture.ZaloRenewer{
		Pool: e.pool, Vault: vault, Client: zalo.NewClient(f.srv.Client(), f.srv.URL, ""), Log: slog.Default(), Now: time.Now,
	}
}

func (e *env) credentialRef(t *testing.T, connection ids.UUID) keyvault.Ref {
	t.Helper()
	var ref string
	if err := e.owner.QueryRow(context.Background(), `SELECT credential_ref FROM channel_connection WHERE id = $1`, connection).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	return keyvault.Ref(ref)
}

func (e *env) sealedPair(t *testing.T, connection ids.UUID) map[string]any {
	t.Helper()
	var ref string
	if err := e.owner.QueryRow(context.Background(), `SELECT credential_ref FROM channel_connection WHERE id = $1`, connection).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	sealed, err := e.vault.Get(context.Background(), ids.From[ids.WorkspaceKind](e.ws), keyvault.Ref(ref))
	if err != nil {
		t.Fatal(err)
	}
	var pair map[string]any
	if err := json.Unmarshal(sealed, &pair); err != nil {
		t.Fatal(err)
	}
	return pair
}

func (e *env) status(t *testing.T, connection ids.UUID) string {
	var s string
	if err := e.owner.QueryRow(context.Background(), `SELECT status FROM channel_connection WHERE id = $1`, connection).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestARenewalRotatesBothTokensAndDestroysTheOldPair(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9991001", "k")
	f := newFakeZalo(t)
	r := e.renewer(f, e.vault)

	oldRef := e.credentialRef(t, a.conn.ID)
	if err := r.RenewWorkspace(context.Background(), e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := e.vault.Get(context.Background(), ids.From[ids.WorkspaceKind](e.ws), oldRef); !errors.Is(err, keyvault.ErrNotFound) {
		t.Errorf("the superseded pair is still in the vault (%v), want it destroyed", err)
	}
	pair := e.sealedPair(t, a.conn.ID)
	if pair["access_token"] != "access-1" || pair["refresh_token"] != "refresh-1" {
		t.Errorf("sealed pair = %v, want both tokens replaced", pair)
	}
	expires, err := time.Parse(time.RFC3339, pair["expires_at"].(string))
	if err != nil || time.Until(expires) < 20*time.Hour {
		t.Errorf("expires_at = %v, %v, want about 25 hours ahead", pair["expires_at"], err)
	}
	if n := e.vault.(interface{ Count() int }).Count(); n != 2 {
		t.Errorf("the vault holds %d secrets, want 2: the old pair must be destroyed", n)
	}
	if err := r.RenewWorkspace(context.Background(), e.ws); err != nil || f.calls.Load() != 1 {
		t.Errorf("a second pass made %d calls in total (%v), want the one: the token is not due", f.calls.Load(), err)
	}
}

func TestARefusedRefreshTokenParksTheConnection(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9991002", "k")
	f := newFakeZalo(t)
	f.answer = func(w http.ResponseWriter, _ int32) {
		_, _ = w.Write([]byte(`{"error":-118,"error_name":"Invalid refresh token","error_description":"expired"}`))
	}
	if err := e.renewer(f, e.vault).RenewWorkspace(context.Background(), e.ws); err != nil {
		t.Fatal(err)
	}
	if got := e.status(t, a.conn.ID); got != "reauth_required" {
		t.Errorf("status = %s, want reauth_required", got)
	}
	body := inbound(a.oaID, "5550001", "msg-r", "hi")
	if rec := e.post(body, sign(a.secret, body)); rec.Code != http.StatusOK || e.rawRows(t) != 1 || e.ingestJobs(t) != 1 {
		t.Errorf("an event for a parked connection got %d with %d rows and %d jobs, want it stored", rec.Code, e.rawRows(t), e.ingestJobs(t))
	}
	if err := e.renewer(f, e.vault).RenewWorkspace(context.Background(), e.ws); err != nil || f.calls.Load() != 1 {
		t.Errorf("a parked connection was tried again: %d calls, %v", f.calls.Load(), err)
	}
}

func TestAFailedVaultWriteParksTheConnectionBecauseZaloSpentTheOldToken(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9991003", "k")
	vault := &failingPut{Vault: e.vault}
	vault.fail.Store(true)
	f := newFakeZalo(t)

	if err := e.renewer(f, vault).RenewWorkspace(context.Background(), e.ws); err == nil {
		t.Fatal("a failed seal was reported as success")
	}
	if got := e.status(t, a.conn.ID); got != "reauth_required" {
		t.Errorf("status = %s, want reauth_required: the old refresh token is spent and the new pair was lost", got)
	}
	if n := e.vault.(interface{ Count() int }).Count(); n != 2 {
		t.Errorf("the vault holds %d secrets, want the connection's own 2 and nothing new", n)
	}
}

// commitThenFail runs the renewal's work, commits it, and then reports an error,
// the way a connection that drops after COMMIT does.
func (e *env) commitThenFail() func(context.Context, func(pgx.Tx) error) error {
	return func(ctx context.Context, fn func(pgx.Tx) error) error {
		if err := database.WithWorkspaceTx(ctx, e.pool, fn); err != nil {
			return err
		}
		return errors.New("the connection dropped after the commit")
	}
}

// rollBack runs the renewal's work and then undoes it, so the row never names
// the new pair although Zalo issued it.
func (e *env) rollBack() func(context.Context, func(pgx.Tx) error) error {
	return func(ctx context.Context, fn func(pgx.Tx) error) error {
		return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
			if err := fn(tx); err != nil {
				return err
			}
			return errors.New("the transaction failed before it committed")
		})
	}
}

func TestACommitThatReportsAnErrorAfterCommittingKeepsTheNamedPairAndRenewsAgain(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9991006", "k")
	f := newFakeZalo(t)
	r := e.renewer(f, e.vault)
	r.Tx = e.commitThenFail()

	if err := r.RenewWorkspace(context.Background(), e.ws); err != nil {
		t.Fatalf("a renewal the row names was reported as failed: %v", err)
	}
	if pair := e.sealedPair(t, a.conn.ID); pair["refresh_token"] != "refresh-1" {
		t.Fatalf("sealed pair = %v, want the renewed pair the row names to still be in the vault", pair)
	}
	if got := e.status(t, a.conn.ID); got != "connected" {
		t.Errorf("status = %s, want connected", got)
	}

	r.Tx = nil
	later := r.Now().Add(20 * time.Hour)
	r.Now = func() time.Time { return later }
	if err := r.RenewWorkspace(context.Background(), e.ws); err != nil {
		t.Fatalf("the next renewal: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.refresh) != 2 || f.refresh[1] != "refresh-1" {
		t.Errorf("Zalo saw refresh tokens %v, want the second renewal to present the first one's new token", f.refresh)
	}
}

func TestARenewalThatRollsBackAfterZaloAnsweredDestroysTheUnnamedPairAndParksTheConnection(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9991007", "k")
	f := newFakeZalo(t)
	r := e.renewer(f, e.vault)
	r.Tx = e.rollBack()

	if err := r.RenewWorkspace(context.Background(), e.ws); err == nil {
		t.Fatal("a rolled-back renewal was reported as success")
	}
	if got := e.status(t, a.conn.ID); got != "reauth_required" {
		t.Errorf("status = %s, want reauth_required", got)
	}
	if n := e.vault.(interface{ Count() int }).Count(); n != 2 {
		t.Errorf("the vault holds %d secrets, want the connection's own 2: the unnamed pair is destroyed", n)
	}
	if pair := e.sealedPair(t, a.conn.ID); pair["access_token"] != "access-9991007" {
		t.Errorf("sealed pair = %v, want the row's own pair left alone", pair)
	}
}

func TestAThrottledRenewalIsRetriedWithTheSamePair(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9991004", "k")
	f := newFakeZalo(t)
	f.answer = func(w http.ResponseWriter, _ int32) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":-32,"error_name":"rate limit"}`))
	}
	if err := e.renewer(f, e.vault).RenewWorkspace(context.Background(), e.ws); err == nil {
		t.Fatal("a throttled renewal was reported as success")
	}
	if got := e.status(t, a.conn.ID); got != "connected" {
		t.Errorf("status = %s, want connected", got)
	}
}

func TestConcurrentRenewalsSpendTheRefreshTokenOnce(t *testing.T) {
	e := setup(t)
	a := e.connect(t, "9991005", "k")
	f := newFakeZalo(t)
	entered, release := make(chan struct{}), make(chan struct{})
	f.answer = func(w http.ResponseWriter, n int32) {
		if n == 1 {
			close(entered)
			<-release
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":"90000"}`))
	}
	r := e.renewer(f, e.vault)
	var wg sync.WaitGroup
	var first error
	wg.Go(func() {
		first = r.RenewWorkspace(context.Background(), e.ws)
	})
	<-entered
	second := r.RenewWorkspace(context.Background(), e.ws)
	close(release)
	wg.Wait()

	if first != nil || second != nil {
		t.Fatalf("passes returned %v and %v, want both clean", first, second)
	}
	if f.calls.Load() != 1 {
		t.Errorf("Zalo saw %d refreshes, want the one: the second pass must skip the locked row", f.calls.Load())
	}
	if got := e.status(t, a.conn.ID); got != "connected" {
		t.Errorf("status = %s, want connected", got)
	}
	if pair := e.sealedPair(t, a.conn.ID); pair["refresh_token"] != "new-refresh" {
		t.Errorf("sealed pair = %v, want the renewed one", pair)
	}
}
