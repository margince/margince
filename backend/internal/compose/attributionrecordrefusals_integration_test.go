// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the record repair REFUSES, and what an Art. 17 erasure takes away from
// its bookkeeping afterwards.
//
// Split from attributionrecords_integration_test.go, which asks where a row
// goes — which store claims each object_type, whether a replay rewrites, what
// a deal write records. These two ask the opposite question: what happens when
// the answer is no. Both arrived as review findings against the record path,
// and both are about a refusal the activity path had already settled and the
// record path silently did not inherit.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// contactLedger reads every column of the repair's bookkeeping about one
// contact that an Art. 17 erasure has an opinion about.
func contactLedger(t *testing.T, e *integration.Env, id ids.UUID) (authorName *string, hash, batchRef string, revision int64) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT source_author_name, payload_hash, batch_ref, source_revision
			  FROM source_attribution_repair
			 WHERE object_type = 'contact' AND object_id = $1`, id).
			Scan(&authorName, &hash, &batchRef, &revision)
	}); err != nil {
		t.Fatalf("reading the contact's ledger row: %v", err)
	}
	return authorName, hash, batchRef, revision
}

// TestAnErasureClearsTheLedgerItHoldsAboutAnErasedContact is the record twin of
// the activity test one file over, and it exists because the record repair
// broke that test's guarantee without touching it.
//
// The ledger holds the author's name a second time and `payload_hash` holds it
// a third, as an unkeyed SHA-256 — and a digest is not anonymity when the
// candidate set is a staff list. The activity path already cleared both. The
// record path writes rows keyed `object_type='contact'` and `'lead'`, and
// nothing reached them: `clearAttributionLedgerNames` was called for activities
// only, so an erased contact's byline stayed plainly readable one table over.
//
// The REVISION must survive, and that half is asserted just as hard. It is how
// the next repair run knows this record was already reached; destroy it and a
// later batch re-attributes the erased contact and writes the name back.
func TestAnErasureClearsTheLedgerItHoldsAboutAnErasedContact(t *testing.T) {
	e := integration.Setup(t)
	h := recordHandlers(e)

	subject := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO contact (id, full_name, source, captured_by, source_system)
		VALUES ($1, 'Mara Kessler', 'hubspot_import', 'human:'||$2, 'hubspot')`, subject, e.AdminUser)
	e.WsExec(t, `
		INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'mara.kessler@example.com', true, 'test', 'human:seed')`, subject)

	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "hubspot-mirror-2026-09-18",
		Rows: []crmcontracts.SourceAttributionRow{
			recordRow(crmcontracts.SourceAttributionRowObjectTypeContact, subject, "Mutaz Suleiman", 7),
		},
	})
	// Without this the assertions below would pass on a repair that recorded
	// nothing at all.
	if _, hash, _, _ := contactLedger(t, e, subject); hash == "" {
		t.Fatal("the attribution recorded no digest, so the assertions below would prove nothing")
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}

	authorName, hash, batchRef, revision := contactLedger(t, e, subject)
	if authorName != nil {
		t.Errorf("the ledger still names %q after the erasure — the name was cleared from the "+
			"contact and left readable in the bookkeeping beside it", *authorName)
	}
	if hash != "" {
		t.Errorf("the ledger kept the digest %q — an unkeyed hash of a name drawn from a staff "+
			"list is re-identifiable, so it goes with the name it was taken over", hash)
	}
	if batchRef != "" {
		t.Errorf("the erasure kept the batch label %q — it is free text an operator types, and no "+
			"wire pattern makes free text safe", batchRef)
	}
	if revision != 7 {
		t.Errorf("the erasure moved the revision to %d, want 7 kept — without it the next repair "+
			"run reads the contact as unattributed and writes the erased name back", revision)
	}
}

// TestARowTheCallerCannotSeeSkipsWithoutTakingTheBatchDown covers what
// auth.EnsureWritable does to a batch when it refuses.
//
// EnsureWritable asks EnsureVisible first, and EnsureVisible applies CAPTURE
// PRIVACY on top of row scope: an `visibility='owner'` contact belonging to
// somebody else answers ErrNotFound. Returned as an error that sentinel would
// propagate out of the store, out of attributeOne, and abort the whole
// per-row transaction — discarding rows already attributed and doing it again
// on every resumed run. It has to be the same skip a missing row gets.
//
// THE PRINCIPAL IS THE AWKWARD PART, and it is the reason this drives the store
// rather than the route. RepairSourceAttribution takes auth.RequireAdmin, while
// an unbounded row scope makes clauseFor return an empty predicate and
// EnsureVisible return nil before capture privacy is ever consulted. So the
// caller that reaches this code is one holding the admin ROLE with a BOUNDED
// row scope, which principal.Permissions allows and the harness's AdminPerms
// does not model.
func TestARowTheCallerCannotSeeSkipsWithoutTakingTheBatchDown(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(InstallationDB(e.Pool))

	// Somebody else's capture-private contact, and one the caller can reach.
	hidden := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO contact (id, full_name, source, captured_by, source_system)
		VALUES ($1, 'Hidden Contact', 'hubspot_import', 'human:'||$2, 'hubspot')`, hidden, e.AdminUser)
	e.MakeCapturePrivate(t, "contact", hidden, e.Rep3)

	// OWNED BY THE CALLER, and the first draft of this fixture was not — which
	// made the control below fail with `permission denied` before capture
	// privacy was ever reached. writeAuthorityPredicate treats an ownerless row
	// as nobody's to change (unownedIsNobodys): a row no one has claimed is not
	// every seat's to rewrite. So a bounded seat needs a row it actually owns
	// for the control to say what it claims.
	reachable := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO contact (id, full_name, owner_id, source, captured_by, source_system)
		VALUES ($1, 'Reachable Contact', $2, 'hubspot_import', 'human:'||$3, 'hubspot')`,
		reachable, e.Rep1, e.AdminUser)

	// Admin by role, bounded by row scope — the one shape that reaches the
	// capture-privacy predicate at all.
	bounded := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"admin"},
		Objects: map[string]principal.ObjectGrant{
			"contact": {Create: true, Read: true, Update: true, Delete: true},
		},
		RowScope: principal.RowScopeTeam,
	})
	name := "Mutaz Suleiman"
	in := storekit.SourceAuthorInput{AuthorName: &name}

	// THE POSITIVE CONTROL FIRST. Without it the assertion below is satisfied by
	// a caller who cannot write any contact, which proves nothing about privacy.
	if err := database.WithWorkspaceTx(bounded, e.Pool, func(tx pgx.Tx) error {
		outcome, reason, err := store.SetContactSourceAuthorTx(
			bounded, tx, integration.ContactIDOf(reachable), in)
		if err != nil {
			return err
		}
		if outcome != storekit.SourceAuthorApplied {
			t.Errorf("the reachable contact answered %q (%s), want applied — this seat cannot "+
				"attribute anything, so the refusal below would prove nothing", outcome, reason)
		}
		return nil
	}); err != nil {
		t.Fatalf("attributing the reachable contact: %v", err)
	}

	// The hidden one: a skip carrying a reason, and NO error — an error would
	// abort the transaction that the rows behind it share.
	if err := database.WithWorkspaceTx(bounded, e.Pool, func(tx pgx.Tx) error {
		outcome, reason, err := store.SetContactSourceAuthorTx(
			bounded, tx, integration.ContactIDOf(hidden), in)
		if err != nil {
			t.Errorf("a contact the caller may not see answered err=%v — returned rather than "+
				"skipped, this aborts the batch and discards every row attributed before it", err)
			return nil
		}
		if outcome != storekit.SourceAuthorSkipped || reason == "" {
			t.Errorf("the hidden contact answered %q (%q), want a skip carrying its reason",
				outcome, reason)
		}
		return nil
	}); err != nil {
		t.Fatalf("driving the hidden contact: %v", err)
	}

	if got := authorOnTable(t, e, `SELECT source_author_name FROM contact WHERE id = $1`, hidden); got != nil {
		t.Errorf("a contact outside the caller's scope was attributed to %v", got)
	}
}
