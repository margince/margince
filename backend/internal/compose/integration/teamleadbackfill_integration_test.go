// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The team_lead backfill against the documents an installation really holds.
// The convergence arms in rbacseedparity prove the SEEDED matrix comes out
// right; these prove what that matrix cannot show: a seeded lead an operator
// had already customised, a role an operator made, and a denial set afterwards.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const teamLeadVersion = "1790516373"

// leadGrant reads a role's stored team_lead grant, nil when it carries no key.
func leadGrant(ctx context.Context, t *testing.T, e *apptest.AppEnv, role string) *bool {
	t.Helper()
	var create *bool
	if err := e.Owner.QueryRow(ctx,
		`SELECT (permissions -> 'objects' -> 'team_lead' ->> 'create')::boolean
		   FROM role WHERE key = $1`, role).Scan(&create); err != nil {
		t.Fatalf("reading %s's team_lead grant: %v", role, err)
	}
	return create
}

// Every seat the key rule let lead keeps leading, even one an operator had
// narrowed, because the key rule never read the document. Every seat it did
// not let lead gets the zero grant, and a role an operator made is left alone.
func TestTheTeamLeadBackfillKeepsExactlyWhoLedBefore(t *testing.T) {
	e := apptest.SetupApp(t)
	ctx := context.Background()
	bootstrapInstallation(t, e)
	rewindTo(ctx, t, e, []string{"team_lead"})
	if _, err := e.Owner.Exec(ctx,
		`UPDATE role SET permissions = jsonb_set(permissions, '{row_scope}', '"own"')
		  WHERE key = 'manager' AND is_system`); err != nil {
		t.Fatalf("narrowing manager: %v", err)
	}

	runCoreMigration(ctx, t, e, teamLeadVersion, true)

	for _, role := range []string{"admin", "management", "manager"} {
		if got := leadGrant(ctx, t, e, role); got == nil || !*got {
			t.Errorf("%s led its teams under the key rule and was not granted team_lead", role)
		}
	}
	for _, role := range []string{"rep", "read_only", "ops"} {
		got := leadGrant(ctx, t, e, role)
		if got == nil {
			t.Errorf("%s carries no team_lead key — the backfill missed it", role)
		} else if *got {
			t.Errorf("%s led nobody under the key rule and was granted team_lead", role)
		}
	}
	if got := leadGrant(ctx, t, e, controlRole); got != nil {
		t.Errorf("an operator's own role was given a team_lead key (%v); it led nobody and must stay untouched", *got)
	}
}

// A rollback keeps an operator's denial. Removing the key on the way down would
// let the next upgrade read its absence as "never decided" and grant it again.
func TestTheTeamLeadRollbackKeepsAnOperatorsDenial(t *testing.T) {
	e := apptest.SetupApp(t)
	ctx := context.Background()
	bootstrapInstallation(t, e)
	if _, err := e.Owner.Exec(ctx,
		`UPDATE role SET permissions = jsonb_set(permissions, '{objects,team_lead}',
		        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
		  WHERE key = 'manager' AND is_system`); err != nil {
		t.Fatalf("denying manager the lead grant: %v", err)
	}

	runCoreMigration(ctx, t, e, teamLeadVersion, false)
	runCoreMigration(ctx, t, e, teamLeadVersion, true)

	if got := leadGrant(ctx, t, e, "manager"); got == nil || *got {
		t.Error("down then up turned the operator's denial on manager back into a grant")
	}
}
