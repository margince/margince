// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"maps"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unitTable runs owner statements around a unit table's name. The lane migrates
// no unit, so the test stands up the table the confirm-first fixture declares.
func unitTable(t *testing.T, table string) func(verb, rest string, args ...any) {
	t.Helper()
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	qualified := pgx.Identifier{"ext", table}.Sanitize()
	exec := func(verb, rest string, args ...any) {
		t.Helper()
		if _, err := owner.Exec(ctx, verb+" "+qualified+" "+rest, args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, dropErr := owner.Exec(context.Background(), `DROP TABLE IF EXISTS `+qualified)
		if err := errors.Join(dropErr, owner.Close(context.Background())); err != nil {
			t.Errorf("removing the unit table: %v", err)
		}
	})
	exec("CREATE TABLE IF NOT EXISTS", "(id uuid PRIMARY KEY)")
	exec("GRANT SELECT ON", "TO margince_app")
	return exec
}

func TestAStagedExtensionCallIsReviewedWithEveryArgumentAndCanBeApproved(t *testing.T) {
	e := integration.Setup(t)
	kinds := extensionStagingKinds(confirmFirstUnit(t))
	if err := approvals.RegisterExtensionKinds(kinds); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := approvals.RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the registered kinds: %v", err)
		}
	})
	kind := kinds[0]
	exec := unitTable(t, kind.TargetTable)
	row := ids.NewV7()
	exec("INSERT INTO", "(id) VALUES ($1)", row)

	permissions := integration.AdminPerms
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects[kind.RbacObject] = principal.ObjectGrant{Read: true, Update: true}
	ctx := e.As(e.AdminUser, nil, permissions)
	svc := NewApprovalsService(e.DB())
	raw, hash, err := diffhash.Canonical([]byte(`{"record_id":"` + row.String() + `","reason":"duplicate"}`))
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Stage(ctx, approvals.StageInput{Kind: kind.Verb, TargetType: kind.TargetTable, TargetID: row, ProposedChange: raw, DiffHash: hash, Summary: "Forget record (demo)"})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.Details == nil {
		t.Fatalf("the staged extension call has no review context: %+v", proposal.Preview)
	}
	shown := map[string]string{"state": proposal.Preview.State}
	for _, detail := range *proposal.Preview.Details {
		shown[detail.Field] = detail.Value.Text
	}
	if shown["state"] != "ready" || shown["summary"] != "Forget record (demo)" ||
		shown["argument:record_id"] != row.String() || shown["argument:reason"] != "duplicate" {
		t.Fatalf("review context = %+v, want the operation and every argument", shown)
	}
	if _, err := svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("approving the reviewed extension call: %v", err)
	}
}
