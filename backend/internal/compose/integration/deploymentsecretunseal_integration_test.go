// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const unsealedLogLine = "removed a sealed deployment credential on this boot"

// removedMailConfig declares the relay password removed and still names a
// username. An operator revoking a leaked password most likely has this file.
func removedMailConfig(t *testing.T, enabled bool) deployconfig.Config {
	t.Helper()
	cfg, err := deployconfig.Parse([]byte("version: 1\nemail:\n  enabled: " + strconv.FormatBool(enabled) + "\n  from_address: ops@example.test\n" +
		"  smtp:\n    host: relay.example.test\n    port: 587\n    username: margince\n    password: ${none}\n"))
	if err != nil {
		t.Fatalf("parsing the deployment file: %v", err)
	}
	return cfg
}

// smtpRefRows counts the stored ref row itself. An absent row and a row
// holding the default read the same through settings.Get, and only the first is
// a removal.
func smtpRefRows(t *testing.T, e *Env) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM setting WHERE key = $1`, identity.SMTPPasswordRefKey).Scan(&n); err != nil {
		t.Fatalf("counting the ref row: %v", err)
	}
	return n
}

// bootWith runs what a serving role runs on this file: the removal at boot,
// then the relay the mailer is built from.
func bootWith(t *testing.T, e *Env, vault keyvault.Vault, cfg deployconfig.Config, log *slog.Logger) mailer.SMTP {
	t.Helper()
	if err := compose.RemoveDeclaredAbsentCredentials(sealCtx(), e.Pool, vault, cfg, log); err != nil {
		t.Fatalf("removing the credentials the deployment declares absent: %v", err)
	}
	relay, err := compose.OperatorMailer(sealCtx(), e.Pool, vault, cfg, config.Static(nil), log)
	if err != nil {
		t.Fatalf("building the relay: %v", err)
	}
	return relay
}

func TestADeclaredRemovalDeletesTheSealedPasswordAndTheMailerSendsWithoutOne(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	if _, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, "SMTP_PASSWORD"),
		config.Static(map[string]string{"SMTP_PASSWORD": "a-leaked-password"}), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("sealing the relay password: %v", err)
	}
	if sealedBlobCount(t, e) != 1 || smtpRefRows(t, e) != 1 {
		t.Fatal("nothing was sealed; the removal below would pass vacuously")
	}

	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, nil))
	relay := bootWith(t, e, vault, removedMailConfig(t, true), log)
	if relay.Username != "" || relay.Password != "" {
		t.Errorf("the mailer would authenticate as %q with a %d-byte password; a removed credential sends without AUTH",
			relay.Username, len(relay.Password))
	}
	if got := sealedBlobCount(t, e); got != 0 {
		t.Errorf("%d sealed blobs survive the removal; the ciphertext must go with its record", got)
	}
	if got := smtpRefRows(t, e); got != 0 {
		t.Errorf("the ref row survives the removal (%d rows)", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = $1`, identity.SMTPPasswordRef.AuditVerb()); got != 2 {
		t.Errorf("%d audit rows, want 2: the seal and the removal", got)
	}
	if strings.Count(logged.String(), unsealedLogLine) != 1 {
		t.Errorf("the boot that removed the credential did not say so once:\n%s", logged.String())
	}

	// The next boot with the same file has nothing left to remove and stays quiet.
	logged.Reset()
	bootWith(t, e, vault, removedMailConfig(t, true), log)
	if strings.Contains(logged.String(), unsealedLogLine) {
		t.Errorf("a boot that removed nothing reported a removal:\n%s", logged.String())
	}

	// And dropping the sentinel afterwards does not bring the leaked password back.
	after, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, ""), config.Static(nil), log)
	if err != nil {
		t.Fatalf("booting with no password key: %v", err)
	}
	if after != "" {
		t.Error("the removed password answered again once the sentinel was dropped")
	}
}

// Leaving the key out is not a removal: the sealed copy keeps answering, and
// the mailer keeps the username it authenticates with.
func TestAnAbsentPasswordKeyStillAuthenticatesWithTheSealedCopy(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	log := slog.New(slog.DiscardHandler)
	if _, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, "SMTP_PASSWORD"),
		config.Static(map[string]string{"SMTP_PASSWORD": "a-relay-password"}), log); err != nil {
		t.Fatalf("sealing the relay password: %v", err)
	}
	relay, err := compose.OperatorMailer(sealCtx(), e.Pool, vault, mailConfig(t, ""), config.Static(nil), log)
	if err != nil {
		t.Fatalf("booting with no password key: %v", err)
	}
	if relay.Username != "margince" || relay.Password != "a-relay-password" {
		t.Errorf("the mailer authenticates as %q with %q, want the username and the sealed password", relay.Username, relay.Password)
	}
	if sealedBlobCount(t, e) != 1 || smtpRefRows(t, e) != 1 {
		t.Error("a boot with no password key removed the sealed copy")
	}
}

// A removal is a revocation, so it holds while outbound mail is switched off.
// The sealed password must not wait for the day mail is switched back on.
func TestADeclaredRemovalHoldsWhileMailIsSwitchedOff(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	log := slog.New(slog.DiscardHandler)
	if _, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, "SMTP_PASSWORD"),
		config.Static(map[string]string{"SMTP_PASSWORD": "a-leaked-password"}), log); err != nil {
		t.Fatalf("sealing the relay password: %v", err)
	}
	if err := compose.RemoveDeclaredAbsentCredentials(sealCtx(), e.Pool, vault, removedMailConfig(t, false), log); err != nil {
		t.Fatalf("booting with mail off and the password declared removed: %v", err)
	}
	if sealedBlobCount(t, e) != 0 || smtpRefRows(t, e) != 0 {
		t.Error("the sealed password survived a boot with mail switched off")
	}
}

// A seal that repoints the row while the removal is deciding keeps its row.
// The removal deleted only the blob it read; the newer ref is not its to
// remove. The seal's transaction holds the lock until the removal waits on it,
// which fixes the order of the two.
func TestARemovalLeavesARefSealedWhileItWasDeciding(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	log := slog.New(slog.DiscardHandler)
	if _, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, "SMTP_PASSWORD"),
		config.Static(map[string]string{"SMTP_PASSWORD": "the-old-password"}), log); err != nil {
		t.Fatalf("sealing the relay password: %v", err)
	}
	ws := ids.From[ids.WorkspaceKind](e.WS)
	fresh, err := vault.Put(sealCtx(), ws, []byte("the-new-password"))
	if err != nil {
		t.Fatalf("sealing the newer password: %v", err)
	}

	sysCtx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "test-concurrent-seal")
	store := compose.NewSettingsStore(e.Pool)
	removed := removedMailConfig(t, true)
	removal := make(chan error, 1)
	err = store.WriteTx(sysCtx, func(tx pgx.Tx) error {
		if err := settings.SetTx(sysCtx, store, tx, identity.SMTPPasswordRef, string(fresh)); err != nil {
			return err
		}
		go func() {
			removal <- compose.RemoveDeclaredAbsentCredentials(sealCtx(), e.Pool, vault, removed, log)
		}()
		waitForAnAdvisoryLockWaiter(t, e)
		return nil
	})
	if err != nil {
		t.Fatalf("repointing the ref: %v", err)
	}
	if err := <-removal; err != nil {
		t.Fatalf("the removal: %v", err)
	}

	ref, err := settings.Get(readCtx(e.WS), store, identity.SMTPPasswordRef)
	if err != nil {
		t.Fatalf("reading the recorded ref: %v", err)
	}
	if ref != string(fresh) {
		t.Fatalf("the ref reads %q after the removal, want the one sealed while it was deciding", ref)
	}
	if _, err := vault.Get(sealCtx(), ws, fresh); err != nil {
		t.Errorf("the newer sealed password is gone: %v", err)
	}
}

// waitForAnAdvisoryLockWaiter returns once some session in this test's
// database is blocked on an advisory lock, which is where the removal waits.
// Each query's round trip paces the loop; the deadline only bounds a failure.
func waitForAnAdvisoryLockWaiter(t *testing.T, e *Env) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := e.Pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND wait_event = 'advisory'`).Scan(&waiting); err != nil {
			t.Fatalf("reading lock waits: %v", err)
		}
		if waiting > 0 {
			return
		}
	}
	t.Fatal("the removal never waited on the ref's lock")
}
