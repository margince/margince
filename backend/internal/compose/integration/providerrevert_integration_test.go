// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What "delete the bought data" must and must not take away.
//
// The action's promise is that a purchase can be undone. The danger in keeping
// it is undoing somebody's work along with it: a field a purchase filled may
// since have been corrected by a colleague, and clearing that would destroy
// real work to satisfy a settings toggle.
//
// Only a database can tell the two apart. The test is an equality inside a
// WHERE clause, and the defect it guards against — a revert that clears on the
// strength of "a purchase once wrote here" — passes every unit test there is.
//
// It lives in this suite rather than beside the store because it needs the REAL
// cross-module binding: integrations cannot name people, so a fixture that
// stubbed the callbacks would prove its own stubs.

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// providerAdmin is the seat that may delete bought data.
//
// The harness's AdminPerms carries no `integrations` grant, so it is widened
// here rather than there: this suite's admin is shared by many files, and a
// grant added for one of them would quietly let the others reach a surface they
// were written to be refused by. Cloned for the same reason.
func providerAdmin(e *Env) context.Context {
	objects := maps.Clone(AdminPerms.Objects)
	objects["integrations"] = principal.ObjectGrant{Read: true, Update: true, Delete: true}
	return e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: AdminPerms.RoleKeys,
		Objects:  objects,
		RowScope: principal.RowScopeAll,
	})
}

// TestDeletingBoughtDataSparesWhatSomebodyElseWrote is the case that decides
// whether this feature is safe to ship.
//
// Two contacts, both filled by the same purchase. Nobody has touched the first
// since. On the second, a rep corrected the title. The revert must clear the
// first and leave the second exactly as the rep left it.
func TestDeletingBoughtDataSparesWhatSomebodyElseWrote(t *testing.T) {
	e := Setup(t)
	store := providerStoreFor(t, e)

	untouched := plantBoughtTitle(t, e, "Founder & CEO")
	corrected := plantBoughtTitle(t, e, "Founder & CEO")
	execAsOwner(t, e, `UPDATE person SET title = 'Managing Director' WHERE id = $1`, corrected)

	if err := store.DeleteProviderData(providerAdmin(e), "surfe"); err != nil {
		t.Fatal(err)
	}

	if got := titleOf(t, e, untouched); got != "" {
		t.Errorf("a bought title survived as %q — the action tells an admin the purchase is gone", got)
	}
	if got := titleOf(t, e, corrected); got != "Managing Director" {
		t.Errorf("title is %q, want the rep's own correction: undoing a purchase must never "+
			"throw away the work somebody did on top of it", got)
	}
}

// TestDeletingBoughtDataForgetsWhatItFilled proves the map goes with the values.
//
// An applied-field row that outlived the revert is a standing claim that a
// purchase sits on a record it has been taken off — and on the next delete it
// would point at whatever somebody has since written there.
func TestDeletingBoughtDataForgetsWhatItFilled(t *testing.T) {
	e := Setup(t)
	store := providerStoreFor(t, e)

	plantBoughtTitle(t, e, "Head of Sales")

	if err := store.DeleteProviderData(providerAdmin(e), "surfe"); err != nil {
		t.Fatal(err)
	}

	var left int
	queryAsOwner(t, e, `SELECT count(*) FROM provider_applied_field`, &left)
	if left != 0 {
		t.Errorf("%d applied-field rows survived; each points at a value the record no longer holds", left)
	}
}

// providerStoreFor builds the store over the real cross-module binding.
func providerStoreFor(t *testing.T, e *Env) *integrations.Store {
	t.Helper()
	reg, err := integrations.NewRegistry(integrations.NewOfflineProvider(0, time.Now))
	if err != nil {
		t.Fatal(err)
	}
	store, err := integrations.NewStore(e.DB(), keyvault.NewMemory(), reg, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return compose.BindProviderDomain(store)
}

// plantBoughtTitle makes a contact whose title a purchase filled, recorded the
// way the applier records it.
func plantBoughtTitle(t *testing.T, e *Env, title string) ids.UUID {
	t.Helper()
	person := ids.NewV7()
	// Its own seeder rather than seedSubject: that one hardcodes one address,
	// so a second call collides on the cross-record uniqueness. This case needs
	// two contacts and no addresses at all.
	execAsOwner(t, e, `
		INSERT INTO person (id, full_name, first_name, title, source, captured_by)
		VALUES ($1, 'Revert Subject', 'Revert', $2, 'manual', 'human:x')`, person, title)

	var run ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			INSERT INTO provider_run
			       (person_id, subject_kind, provider, trigger, state, connection_version,
			        connection_epoch, configuration_snapshot, requested_categories,
			        input_fingerprint, external_correlation_id, completed_at)
			VALUES ($1, 'person', 'surfe', 'manual', 'completed', 1, 1, '{}'::jsonb,
			        ARRAY['current_employment'], $2, gen_random_uuid(), now())
			RETURNING id`, person, "fp-revert-"+person.String()).Scan(&run)
	}); err != nil {
		t.Fatal(err)
	}
	execAsOwner(t, e, `
		INSERT INTO provider_applied_field
		       (person_id, run_id, provider, target_table, target_field, applied_value, captured_by)
		VALUES ($1, $2, 'surfe', 'person', 'title', $3, 'connector:surfe')`,
		person, run, title)
	return person
}

// titleOf reads a contact's title, empty when it carries none.
func titleOf(t *testing.T, e *Env, person ids.UUID) string {
	t.Helper()
	var title *string
	queryAsOwner(t, e, `SELECT title FROM person WHERE id = $1`, &title, person)
	if title == nil {
		return ""
	}
	return *title
}

// execAsOwner runs one statement in the installation's workspace.
//
//craft:ignore naked-any the args and destination forward straight to pgx, whose own Exec/Scan take any
func execAsOwner(t *testing.T, e *Env, statement string, args ...any) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), statement, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// queryAsOwner reads one value in the installation's workspace.
//
//craft:ignore naked-any same: this is pgx's own signature, one call deeper
func queryAsOwner(t *testing.T, e *Env, statement string, into any, args ...any) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), statement, args...).Scan(into)
	}); err != nil {
		t.Fatal(err)
	}
}
