// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A provider run enriches a contact a colleague captured privately.
//
// Capture privacy answers an owner-private row to its owner alone. A provider
// run has no owner and no human behind it, and buys for the installation.
//
// Without the widening in platform/auth the run pays the provider and then
// cannot write what it bought. The money leaves, the record stays empty, and
// no screen says why.
//
// It needs the real cross-module binding. The module's own fixtures bind no
// claim writer, and the page fixtures write as an admin.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestAProviderRunEnrichesAContactCapturedPrivatelyByAColleague(t *testing.T) {
	e := Setup(t)
	connectProvider(t, e)
	// The store is built here rather than through providerStoreFor because the
	// run needs a credential, and only the holder of the vault can seal one.
	reg, err := integrations.NewRegistry(integrations.NewOfflineProvider(0, time.Now))
	if err != nil {
		t.Fatal(err)
	}
	vault := keyvault.NewMemory()
	raw, err := integrations.NewStore(e.DB(), vault, reg, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// A no-op enqueue: this test drives submit and poll itself, and QueueRun
	// refuses to commit a run no job would ever pick up.
	store := compose.BindProviderDomain(raw).WithSubmitEnqueue(
		func(context.Context, pgx.Tx, string, string) error { return nil })
	ref, err := vault.Put(context.Background(), ids.From[ids.WorkspaceKind](e.WS), []byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `
		UPDATE provider_connection
		   SET credential_ref = $1, execution_epoch = execution_epoch + 1
		 WHERE provider = 'surfe'`, ref)

	contact := ids.NewV7()
	execAsOwner(t, e, `
		INSERT INTO contact (id, full_name, first_name, last_name, source, captured_by)
		VALUES ($1, 'Petra Private', 'Petra', 'Private', 'manual', 'human:x')`, contact)
	// A name and a company, which is what the run demands before it spends a
	// call. Without one the run skips as `no_identifiers`, and never reaches
	// the question this test asks.
	company := ids.NewV7()
	execAsOwner(t, e, `
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Private Holdings GmbH', 'manual', 'human:x')`, company)
	execAsOwner(t, e, `
		INSERT INTO relationship
		       (id, kind, contact_id, company_id, source, captured_by, is_current_primary)
		VALUES ($1, 'employment', $2, $3, 'manual', 'human:x', true)`,
		ids.NewV7(), contact, company)
	// The row a colleague keeps to themselves: answerable to its owner alone,
	// and that owner is not the seat any run acts as.
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, contact, e.Rep1)

	// The OWNER queues it, which is the only seat that can: the row gate hides
	// their private contact from everyone else, this suite's admin included.
	owner := e.As(e.Rep1, nil, AdminPerms)
	run, queueErr := store.QueueRun(owner, provider.QueueInput{
		ContactID: contact.String(), Provider: "surfe", Trigger: "manual",
		Categories: []provider.Category{"professional_email", "mobile"},
	})
	if queueErr != nil {
		t.Fatalf("queue a run for a privately captured contact: %v", queueErr)
	}
	if err := store.ExecuteSubmit(owner, run.ID); err != nil {
		t.Fatalf("submit: %v", err)
	}
	// The offline provider completes on the first poll, so this drives the
	// terminal commit and the hand-off that writes under the connector.
	// The sweep's own principal is irrelevant: actingForProvider replaces it
	// with the run's connector before anything is written.
	if err := store.RunDueSweep(e.Admin()); err != nil {
		t.Fatalf("the due sweep could not land a purchase on an owner-private contact: %v", err)
	}

	var claims int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM contact_provider_claim WHERE contact_id = $1`,
			contact).Scan(&claims)
	}); err != nil {
		t.Fatal(err)
	}
	if claims == 0 {
		var st, unwritten, skip string
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT state, coalesce(claims_unwritten::text, '-'), coalesce(skip_reason, '-')
				   FROM provider_run WHERE id = $1`, run.ID).Scan(&st, &unwritten, &skip)
		}); err != nil {
			t.Fatalf("reading the run back: %v", err)
		}
		t.Fatalf("stored nothing; state=%s claims_unwritten=%s skip_reason=%s", st, unwritten, skip)
	}

	var state string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT state FROM provider_run WHERE id = $1`, run.ID).Scan(&state)
	}); err != nil {
		t.Fatal(err)
	}
	if state != string(provider.RunCompleted) {
		t.Errorf("the run is %q, want completed", state)
	}
}
