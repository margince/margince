// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// One human, one provider, one row — but not necessarily one mailbox. A contact
// who connects a second account over the first is not resuming the first: the
// watermark and the import history belong to the account, and the row is only
// where they happen to live.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// accountBoundConnector names its account from the sealed bundle, so a test
// reconnects "as somebody else" simply by granting different bytes. Its Sync
// yields a watermark, which is what the rebind has to throw away.
type accountBoundConnector struct {
	*pagedConnector
	// duringPage runs where the provider would be answering, so a test can
	// reconnect while a backfill page is out — the race a reauth actually runs.
	duringPage func()
}

func (c *accountBoundConnector) AccountLabel(auth connector.Auth) (string, error) {
	return string(auth), nil
}

func (c *accountBoundConnector) Sync(context.Context, connector.Auth, connector.Cursor, connector.Sink) (connector.Cursor, error) {
	return connector.Cursor(`{"email":"owner@myco.example"}`), nil
}

func (c *accountBoundConnector) BackfillPage(ctx context.Context, auth connector.Auth, after time.Time, pageToken string, sink connector.Sink) (connector.BackfillPageResult, error) {
	if c.duringPage != nil {
		c.duringPage()
	}
	return c.pagedConnector.BackfillPage(ctx, auth, after, pageToken, sink)
}

// startImportReconnectingMidPage opens a run over account, then reconnects as
// reconnectAs while the page is out at the provider, and returns the run's row
// afterwards. It is the whole race in one helper: the only difference between a
// reauth and a rebind is whether the second grant names the same mailbox.
func startImportReconnectingMidPage(t *testing.T, e *integration.SearchEnv, account, reconnectAs string) (status string, scanned int, cursor []byte) {
	t.Helper()
	seedCaptureRole(t, e)
	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	fake := &accountBoundConnector{pagedConnector: &pagedConnector{messages: 25, pageSize: 10}}
	registry.Register(fake)
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth(account)); err != nil {
		t.Fatalf("connecting %s: %v", account, err)
	}
	run, err := registry.StartBackfill(grantCtx, capturemod.ProviderMailbox("gmail"), ids.From[ids.UserKind](e.Rep1), 6, connector.BackfillEstimate{Messages: 25}, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	fake.duringPage = func() {
		if _, err := registry.Connect(grantCtx, "gmail", connector.Auth(reconnectAs)); err != nil {
			t.Errorf("mid-page reconnect as %s: %v", reconnectAs, err)
		}
	}
	if _, _, _, err := registry.RunBackfillStep(context.Background(), run.ID); err != nil {
		t.Fatalf("RunBackfillStep across a reconnect: %v, want a clean return", err)
	}
	status, scanned, _, cursor = readBackfillRow(t, e, run.ID)
	return status, scanned, cursor
}

// A reauth is not a rebind. The reauth_required banner asks its human to
// reconnect the SAME mailbox, and the page already out at the provider belongs
// to the same account and the same import it always did — fencing it off would
// report the import the human was told to repair as "cancelled".
func TestAReauthOfTheSameAccountLetsAnInFlightPageCommit(t *testing.T) {
	e := integration.SetupSearch(t)
	status, scanned, cursor := startImportReconnectingMidPage(t, e, "same@example.com", "same@example.com")
	if status != "running" {
		t.Fatalf("status = %s, want running — a routine reauth must not end the import it was meant to keep alive", status)
	}
	if scanned != 10 || len(cursor) == 0 {
		t.Fatalf("the page was fenced off by its own human's reauth: scanned=%d cursor=%q", scanned, cursor)
	}
}

// readConnectionAccount reads what a connection is currently bound to and where
// it is up to.
func readConnectionAccount(t *testing.T, e *integration.SearchEnv, connID ids.UUID) (label *string, cursor []byte) {
	t.Helper()
	err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `
			SELECT account_label, sync_cursor FROM capture_connection WHERE id = $1`, connID).
			Scan(&label, &cursor)
	})
	if err != nil {
		t.Fatal(err)
	}
	return label, cursor
}

// connectAndSync grants the provider under the given account and pulls once, so
// the connection carries a real watermark before anything reconnects.
func connectAndSync(t *testing.T, registry *capturemod.Registry, e *integration.SearchEnv, account string) ids.UUID {
	t.Helper()
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "gmail", connector.Auth(account))
	if err != nil {
		t.Fatalf("connecting %s: %v", account, err)
	}
	if err := registry.SyncOnce(context.Background(), connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if _, cursor := readConnectionAccount(t, e, connID); len(cursor) == 0 {
		t.Fatal("the fixture precondition does not hold: the first account left no watermark to inherit")
	}
	return connID
}

func TestReconnectingTheSameAccountKeepsItsWatermark(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	registry.Register(&accountBoundConnector{pagedConnector: &pagedConnector{messages: 5, pageSize: 10}})

	connID := connectAndSync(t, registry, e, "same@example.com")

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("same@example.com")); err != nil {
		t.Fatalf("re-granting the same account: %v", err)
	}

	if _, cursor := readConnectionAccount(t, e, connID); len(cursor) == 0 {
		t.Fatal("re-granting the same mailbox threw away its watermark — a routine reauth would re-read the whole history")
	}
}

func TestReconnectingTheSameAccountKeepsItsSignatureAnswer(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	registry.Register(&accountBoundConnector{pagedConnector: &pagedConnector{messages: 5, pageSize: 10}})

	connID := connectAndSync(t, registry, e, "same@example.com")
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	off := false
	if _, err := registry.SetSignatureEnrichment(grantCtx, capturemod.ProviderMailbox("gmail"), &off); err != nil {
		t.Fatalf("switching the mailbox off: %v", err)
	}

	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("same@example.com")); err != nil {
		t.Fatalf("re-granting the same account: %v", err)
	}

	// A routine reauth is the same contact and the same mailbox. Clearing their
	// answer would quietly re-enable reading mail they asked us not to read.
	answer := readSignatureAnswer(t, e, connID)
	if answer == nil || *answer {
		t.Fatalf("signature_enrich_enabled = %v, want the false this mailbox chose", answer)
	}
}

// readSignatureAnswer reads one connection's own answer straight from the row.
func readSignatureAnswer(t *testing.T, e *integration.SearchEnv, connID ids.UUID) *bool {
	t.Helper()
	var answer *bool
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT signature_enrich_enabled FROM capture_connection WHERE id = $1`, connID).Scan(&answer)
	}); err != nil {
		t.Fatalf("reading the connection's signature answer: %v", err)
	}
	return answer
}
