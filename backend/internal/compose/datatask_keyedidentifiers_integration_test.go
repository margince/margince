// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
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

// useRootKey binds a worker configuration holding key, as a booted worker
// does; an empty key binds one with none.
func useRootKey(t *testing.T, key string) {
	t.Helper()
	var environ []string
	if key != "" {
		environ = append(environ, "MARGINCE_KEYVAULT_ROOT_KEY="+key)
	}
	UseConfig(bootConfigForTest(t, config.RoleWorker, environ...))
	t.Cleanup(func() { UseConfig(nil) })
}

func keyedRecipient(t *testing.T, e *integration.Env, name, address string) ids.UUID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(contact.Id)
}

func storedAddressRefused(t *testing.T, e *integration.Env, address string) bool {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return storekit.RefuseErasedEmail(e.Admin(), tx, "email", address)
	})
	var erased *storekit.ErasedIdentifierError
	if err != nil && !errors.As(err, &erased) {
		t.Fatal(err)
	}
	return erased != nil
}

func TestAnAddressErasedBeforeTheKeyStaysRefusedThroughTheRekey(t *testing.T) {
	e := integration.Setup(t)
	useRootKey(t, "")
	const address = "Erased.Subject@rekey.test"
	subject := keyedRecipient(t, e, "Erased Subject", address)
	if err := ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), subject, "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := identity.NewService(e.Pool).Login(e.Admin(), "nobody@rekey.test", "wrong", "test"); err == nil {
		t.Fatal("a login for an unknown address succeeded")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM erasure_suppression WHERE key_id IS NULL`); n == 0 {
		t.Fatal("an erasure without a root key wrote no unkeyed row")
	}

	useRootKey(t, firstRootKey)
	if !storedAddressRefused(t, e, address) {
		t.Fatal("an address erased before the key was not refused before the rekey")
	}
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
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
	if !storedAddressRefused(t, e, strings.ToLower(address)) {
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
	later := keyedRecipient(t, e, "Later Subject", "later@rekey.test")
	if err := ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), later, "test"); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM erasure_suppression WHERE key_id IS NULL`); n != 0 || !storedAddressRefused(t, e, "later@rekey.test") {
		t.Fatalf("an erasure under the key wrote %d unkeyed rows, or its address is not refused", n)
	}
	if err := storekit.RefuseForeignSuppressionKey(e.Admin(), e.Pool); err != nil {
		t.Fatalf("the key that made the list was refused: %v", err)
	}
	for _, other := range []string{secondRootKey, ""} {
		useRootKey(t, other)
		if err := storekit.RefuseForeignSuppressionKey(e.Admin(), e.Pool); err == nil {
			t.Fatalf("a process with root key %q would have started over a list it cannot match", other)
		}
	}
}

func TestAPreferenceLinkMailedBeforeTheKeyKeepsOpeningAfterItIsHashed(t *testing.T) {
	e := integration.Setup(t)
	useRootKey(t, "")
	const address = "reader@rekey.test"
	recipient := keyedRecipient(t, e, "Newsletter Reader", address)
	store := consent.NewStore(e.DB())
	mailed, found, err := store.PreferenceTokenForEmail(e.Admin(), address)
	if err != nil || !found || !strings.HasPrefix(mailed, "pref_") {
		t.Fatalf("minting without a root key = (%q, %v, %v), want a stored pref_ link", mailed, found, err)
	}

	useRootKey(t, firstRootKey)
	opens := func(link string) bool {
		t.Helper()
		ref, err := store.ResolvePreferenceToken(e.Admin(), link)
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
	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, keyedIdentifiersDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM preference_token WHERE token IS NOT NULL`); n != 0 {
		t.Fatalf("%d preference links are still stored after the hashing", n)
	}
	if !opens(mailed) {
		t.Fatal("the mailed link stopped opening once its row held only its hash")
	}
	if _, err := store.ResolveWithdrawalToken(e.Admin(), mailed); err != nil {
		t.Fatalf("the mailed link no longer withdraws: %v", err)
	}

	signed, _, err := store.PreferenceTokenForEmail(e.Admin(), address)
	if err != nil || !strings.HasPrefix(signed, "pref2_") || !opens(signed) {
		t.Fatalf("the next send's link = (%q, %v), want a signed link that opens", signed, err)
	}
	again, _, err := store.PreferenceTokenForEmail(e.Admin(), address)
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
	useRootKey(t, secondRootKey)
	if opens(signed) {
		t.Fatal("a link signed under another root key opened the preference centre")
	}
}

func TestAKeyedExportRemovesTheUnkeyedJournalAndStillCatchesARestore(t *testing.T) {
	e := integration.Setup(t)
	store := blobstore.NewMemory()
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(store)
	useRootKey(t, "")
	seedSuppressedEmail(t, resurrectedAddress)
	unkeyedEntry := "erasure-suppression/email/" + storekit.SuppressionHash(resurrectedAddress)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatal(err)
	}

	useRootKey(t, firstRootKey)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(e.Admin(), unkeyedEntry); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("the unkeyed journal entry survived a keyed export: %v", err)
	}

	clearSuppressionList(t)
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)
	took, err := eraser.ReapplySuppressions(e.Admin())
	if err != nil || took != 1 {
		t.Fatalf("ReapplySuppressions = (%d, %v), want the keyed journal to catch the restore", took, err)
	}
	if live := liveEmails(t, e, resurrected); live != 0 {
		t.Fatalf("the restored subject kept %d address(es)", live)
	}
}
