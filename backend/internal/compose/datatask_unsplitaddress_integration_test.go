// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const oneLineAddress = "Example Street 4\n12345 Example City\nGermany"

// oneLineAddressCompany lands a company through a site read, then stores its
// address the way the fill before the splitter did: all of it in line 1.
func oneLineAddressCompany(t *testing.T, e *integration.Env, conn *pgx.Conn) ids.UUID {
	t.Helper()
	source := "https://one-line-" + ids.NewV7().String() + ".example.test/imprint"
	id, err := e.Contacts.ApplyColdStartProfile(e.Admin(), contacts.ApplyColdStartProfileInput{SourceURL: source, Fields: []contacts.ColdStartFieldInput{{
		Field: "registered_address", Value: oneLineAddress, EvidenceSnippet: "Registered office: " + oneLineAddress, SourceURL: source, Confidence: 0.9,
	}}})
	if err != nil {
		t.Fatalf("applying the read: %v", err)
	}
	if _, err := conn.Exec(context.Background(), `UPDATE company SET address_line1 = $2, address_line2 = NULL, address_region = NULL,
	       address_postal_code = NULL, address_city = NULL, address_country = NULL WHERE id = $1`, id, oneLineAddress); err != nil {
		t.Fatal(err)
	}
	return id.UUID
}

func splitCompanies(t *testing.T, e *integration.Env, id ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM company WHERE id = $1 AND address_line1 = 'Example Street 4'
		AND address_postal_code = '12345' AND address_city = 'Example City' AND address_country = 'DE'`, id)
}

func TestTheUnsplitAddressTaskSplitsThroughTheExecutorAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	first, second := oneLineAddressCompany(t, e, conn), oneLineAddressCompany(t, e, conn)

	task := unsplitAddressDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runMachineAddressTask(ctx, t, e, task)

	for _, id := range []ids.UUID{first, second} {
		if splitCompanies(t, e, id) != 1 {
			t.Errorf("company %s was not split into its parts", id)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = ANY($1)
		AND after ->> 'address_city' = 'Example City'`, []ids.UUID{first, second}); got != 2 {
		t.Errorf("split audit rows = %d, want one per company", got)
	}

	step := task.Steps[0]
	in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { var err error; out, err = step.Batch(ctx, tx, in); return err })
		if err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 || out.Scanned > 1 {
			t.Fatalf("a replay batch scanned %d and changed %d, want at most 1 and 0", out.Scanned, out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}

func TestTheUnsplitAddressTaskRollsItsSplitsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	id := oneLineAddressCompany(t, e, conn)

	task := unsplitAddressDataTask()
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runMachineAddressTask(ctx, t, e, task)

	if splitCompanies(t, e, id) != 0 {
		t.Error("a split survived the rolled-back batch")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM company WHERE id = $1 AND address_line1 = $2`, id, oneLineAddress); got != 1 {
		t.Error("the rolled-back batch left the line changed")
	}
}
