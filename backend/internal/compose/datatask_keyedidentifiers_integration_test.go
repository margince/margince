// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Two root keys, 32 bytes of 0x01 and of 0x02.
const (
	firstRootKey  = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
	secondRootKey = "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="
)

// underRootKey is ctx serving a worker whose configuration holds key, as a
// booted worker's does; an empty key is one with none.
func underRootKey(ctx context.Context, t *testing.T, key string) context.Context {
	t.Helper()
	var environ []string
	if key != "" {
		environ = append(environ, "MARGINCE_KEYVAULT_ROOT_KEY="+key)
	}
	return ConfigContext(ctx, bootConfigForTest(t, config.RoleWorker, environ...))
}

func keyedRecipient(t *testing.T, e *integration.Env, name, address string) ids.UUID {
	t.Helper()
	return recipientIn(e.Admin(), t, e, name, address)
}

// recipientIn is keyedRecipient created in the program ctx serves.
func recipientIn(ctx context.Context, t *testing.T, e *integration.Env, name, address string) ids.UUID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(contact.Id)
}

func storedAddressRefused(ctx context.Context, t *testing.T, e *integration.Env, address string) bool {
	t.Helper()
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return storekit.RefuseErasedEmail(ctx, tx, "email", address)
	})
	var erased *storekit.ErasedIdentifierError
	if err != nil && !errors.As(err, &erased) {
		t.Fatal(err)
	}
	return erased != nil
}

func TestAnAddressErasedBeforeTheKeyStaysRefusedThroughTheRekey(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	unkeyed := underRootKey(e.Admin(), t, "")
	const address = "Erased.Subject@rekey.test"
	subject := recipientIn(unkeyed, t, e, "Erased Subject", address)
	if err := ComposedEraser(e.Pool, nil).EraseContact(unkeyed, subject, "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := identity.NewService(e.Pool).Login(unkeyed, "nobody@rekey.test", "wrong", "test"); err == nil {
		t.Fatal("a login for an unknown address succeeded")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM erasure_suppression WHERE key_id IS NULL`); n == 0 {
		t.Fatal("an erasure without a root key wrote no unkeyed row")
	}

	keyed := underRootKey(e.Admin(), t, firstRootKey)
	if !storedAddressRefused(keyed, t, e, address) {
		t.Fatal("an address erased before the key was not refused before the rekey")
	}
	ctx := principal.SystemActing(keyed, "system:datatask-test")
	task := keyedIdentifiersDataTask()
	runMachineAddressTask(ctx, t, e, task)

	for _, unkeyed := range []string{
		`SELECT count(*) FROM erasure_suppression WHERE key_id IS NULL`,
		`SELECT count(*) FROM system_log WHERE action = 'login' AND NOT detail ? 'email_key_id'`,
	} {
		if n := e.WsCount(t, unkeyed); n != 0 {
			t.Fatalf("%d rows remain unkeyed after the rekey: %s", n, unkeyed)
		}
	}
	if !storedAddressRefused(keyed, t, e, strings.ToLower(address)) {
		t.Fatal("the rekey let an erased address be stored again")
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1000, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("a replay re-keyed %d rows again", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	later := recipientIn(keyed, t, e, "Later Subject", "later@rekey.test")
	if err := ComposedEraser(e.Pool, nil).EraseContact(keyed, later, "test"); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM erasure_suppression WHERE key_id IS NULL`); n != 0 || !storedAddressRefused(keyed, t, e, "later@rekey.test") {
		t.Fatalf("an erasure under the key wrote %d unkeyed rows, or its address is not refused", n)
	}
	if err := storekit.RefuseForeignSuppressionKey(keyed, e.Pool); err != nil {
		t.Fatalf("the key that made the list was refused: %v", err)
	}
	for _, other := range []string{secondRootKey, ""} {
		if err := storekit.RefuseForeignSuppressionKey(underRootKey(e.Admin(), t, other), e.Pool); err == nil {
			t.Fatalf("a process with root key %q would have started over a list it cannot match", other)
		}
	}
}

func TestAPreferenceLinkMailedBeforeTheKeyKeepsOpeningAfterItIsHashed(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	unkeyed := underRootKey(e.Admin(), t, "")
	const address = "reader@rekey.test"
	recipient := recipientIn(unkeyed, t, e, "Newsletter Reader", address)
	store := consent.NewStore(e.DB())
	mailed, found, err := store.PreferenceTokenForEmail(unkeyed, address)
	if err != nil || !found || !strings.HasPrefix(mailed, "pref_") {
		t.Fatalf("minting without a root key = (%q, %v, %v), want a stored pref_ link", mailed, found, err)
	}

	ctx := underRootKey(e.Admin(), t, firstRootKey)
	opens := func(link string) bool {
		t.Helper()
		ref, err := store.ResolvePreferenceToken(ctx, link)
		if errors.Is(err, apperrors.ErrNotFound) {
			return false
		}
		if err != nil {
			t.Fatal(err)
		}
		return ref.ContactID.UUID == recipient
	}
	if !opens(mailed) {
		t.Fatal("the mailed link stopped opening when the key was bound")
	}
	runMachineAddressTask(principal.SystemActing(ctx, "system:datatask-test"), t, e, keyedIdentifiersDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM preference_token WHERE token IS NOT NULL`); n != 0 {
		t.Fatalf("%d preference links are still stored after the hashing", n)
	}
	if !opens(mailed) {
		t.Fatal("the mailed link stopped opening once its row held only its hash")
	}
	if _, err := store.ResolveWithdrawalToken(ctx, mailed); err != nil {
		t.Fatalf("the mailed link no longer withdraws: %v", err)
	}

	signed, _, err := store.PreferenceTokenForEmail(ctx, address)
	if err != nil || !strings.HasPrefix(signed, "pref2_") || !opens(signed) {
		t.Fatalf("the next send's link = (%q, %v), want a signed link that opens", signed, err)
	}
	again, _, err := store.PreferenceTokenForEmail(ctx, address)
	if err != nil || again != signed {
		t.Fatalf("the following send handed out %q, want the same link %q", again, signed)
	}
	forged := signed[:len(signed)-2] + "AA"
	if forged == signed {
		forged = signed[:len(signed)-2] + "BB"
	}
	if opens(forged) {
		t.Fatal("a link with a changed MAC opened the preference centre")
	}
	ctx = underRootKey(e.Admin(), t, secondRootKey)
	if opens(signed) {
		t.Fatal("a link signed under another root key opened the preference centre")
	}
}

func TestAKeyedExportRemovesTheUnkeyedJournalAndStillCatchesARestore(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	store := blobstore.NewMemory()
	eraser := eraserWithGraph(InstallationDB(e.Pool)).WithBlobstore(store)
	unkeyed := underRootKey(e.Admin(), t, "")
	seedSuppressedEmail(t, resurrectedAddress)
	unkeyedEntry := "erasure-suppression/email/" + storekit.SuppressionHash(unkeyed, resurrectedAddress)
	if _, err := eraser.ExportSuppressions(unkeyed); err != nil {
		t.Fatal(err)
	}

	keyed := underRootKey(e.Admin(), t, firstRootKey)
	if _, err := eraser.ExportSuppressions(keyed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(keyed, unkeyedEntry); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("the unkeyed journal entry survived a keyed export: %v", err)
	}

	clearSuppressionList(t)
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)
	took, err := eraser.ReapplySuppressions(keyed)
	if err != nil || took != 1 {
		t.Fatalf("ReapplySuppressions = (%d, %v), want the keyed journal to catch the restore", took, err)
	}
	if live := liveEmails(t, e, resurrected); live != 0 {
		t.Fatalf("the restored subject kept %d address(es)", live)
	}
}

func TestAKeyedAnonymizedContactAloneRefusesAnotherRootKey(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	anonymized := recipientIn(underRootKey(e.Admin(), t, ""), t, e, "Anonymized Subject", "anonymized@rekey.test")
	// The hashes an anonymize kept before the installation had a root key.
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE contact
		SET anonymized_address_hashes = ARRAY[$2::text] WHERE id = $1`,
		anonymized, strings.Repeat("ab", 32)); err != nil {
		t.Fatal(err)
	}
	keyed := underRootKey(e.Admin(), t, firstRootKey)
	runMachineAddressTask(principal.SystemActing(keyed, "system:datatask-test"), t, e, keyedIdentifiersDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM erasure_suppression WHERE key_id IS NOT NULL`) +
		e.WsCount(t, `SELECT count(*) FROM communication_suppression WHERE address_key_id IS NOT NULL`); n != 0 {
		t.Fatalf("%d keyed rows outside the contact table; the test needs the contact alone", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE anonymized_address_key_id IS NOT NULL`); n != 1 {
		t.Fatalf("%d contacts hold keyed hashes, want the anonymized one", n)
	}
	if err := storekit.RefuseForeignSuppressionKey(underRootKey(e.Admin(), t, secondRootKey), e.Pool); err == nil {
		t.Fatal("a process under another root key would start over kept hashes it cannot match")
	}
}

func TestTheRekeyQueuesTheJournalExportThatRemovesUnkeyedNames(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	store := blobstore.NewMemory()
	exporter := suppressionJournalExporterFor(e.Pool, store, slog.New(slog.DiscardHandler))
	unkeyed := underRootKey(t.Context(), t, "")
	seedSuppressedEmail(t, resurrectedAddress)
	unkeyedEntry := "erasure-suppression/email/" + storekit.SuppressionHash(unkeyed, resurrectedAddress)
	if err := exporter.Work(unkeyed, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(t.Context(), unkeyedEntry); err != nil {
		t.Fatalf("the export without a key wrote no unkeyed entry: %v", err)
	}

	keyed := underRootKey(e.Admin(), t, firstRootKey)
	client, err := river.NewClient(riverpgxv5.New(e.Pool), &river.Config{
		Logger: slog.New(slog.DiscardHandler), SkipUnknownJobCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := rivertest.WorkContext(principal.SystemActing(keyed, "system:datatask-test"), client)
	runMachineAddressTask(ctx, t, e, keyedIdentifiersDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind = $1 AND state = 'available'`,
		SuppressionJournalArgs{}.Kind()); n != 1 {
		t.Fatalf("the finished re-key queued %d journal exports, want one", n)
	}

	if err := exporter.Work(underRootKey(t.Context(), t, firstRootKey), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(t.Context(), unkeyedEntry); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("an unkeyed journal name survived the queued export: %v", err)
	}
}
