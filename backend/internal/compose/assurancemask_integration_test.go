// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A finding quotes its deal's money, so the list of findings is one more way to
// a figure a field mask on deal.amount_minor withholds. The mask is rendered
// into the statement, so only a real database can say whether it holds.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAMaskedReaderIsListedNoFindingMoneyOnADealTheyCannotChange(t *testing.T) {
	t.Parallel()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` " +
			"(integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	wsTyped := ids.New[ids.WorkspaceKind]()
	mine, theirs := ids.NewV7(), ids.NewV7()
	exec(`INSERT INTO workspace (id) VALUES ($1)`, wsTyped.UUID)
	for _, user := range []ids.UUID{mine, theirs} {
		exec(`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
			user, "rep-"+user.String()+"@mask.test")
	}
	pipeline, stage := ids.NewV7(), ids.NewV7()
	exec(`INSERT INTO pipeline (id, name) VALUES ($1, 'Mask')`, pipeline)
	exec(`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`,
		stage, pipeline)

	// On each deal, one money finding and one date finding, as the scan writes them.
	myDeal, theirDeal := ids.NewV7(), ids.NewV7()
	for deal, dealOwner := range map[ids.UUID]ids.UUID{myDeal: mine, theirDeal: theirs} {
		exec(`INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, amount_minor, currency, source, captured_by)
			 VALUES ($1, 'Deal', 'open', $2, $3, $4, 500000, 'EUR', 'seed', 'test')`,
			deal, dealOwner, pipeline, stage)
		exec(`INSERT INTO assurance_exception
			     (logical_key, type, subject_kind, subject_id, severity, owner_id,
			      affected_minor, currency, claim, observed, captured_by)
			 VALUES ($1, $2, 'deal', $3, 'medium', $4, 200000, 'EUR',
			         '{"amount_minor":500000}', '{"offer_total":300000}', 'test')`,
			assurance.TypeAmountVsOffer+":"+deal.String(), assurance.TypeAmountVsOffer, deal, dealOwner)
		exec(`INSERT INTO assurance_exception
			     (logical_key, type, subject_kind, subject_id, severity, owner_id,
			      affected_minor, currency, claim, observed, captured_by)
			 VALUES ($1, $2, 'deal', $3, 'high', $4, 500000, 'EUR',
			         '{"expected_close":"2026-04-30"}', '{"as_of":"2026-05-14"}', 'test')`,
			assurance.TypeClosePast+":"+deal.String(), assurance.TypeClosePast, deal, dealOwner)
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)

	// RowScopeTeam, not All: auth.Unbounded skips masks under row_scope=all.
	repCtx := principal.WithWorkspaceID(context.Background(), wsTyped.UUID)
	repCtx = principal.WithCorrelationID(repCtx, ids.NewV7())
	repCtx = principal.WithActor(repCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + mine.String(), UserID: mine,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true},
				"deal":     {Read: true, Update: true},
			},
			RowScope: principal.RowScopeTeam,
			FieldMasks: []principal.FieldMask{{
				Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority,
			}},
		},
	})

	var found []assurance.Exception
	if err := database.WithWorkspaceTx(repCtx, database.BindTo(pool, wsTyped).Pool(),
		func(tx pgx.Tx) error {
			found, err = AssuranceExceptions(repCtx, tx)
			return err
		}); err != nil {
		t.Fatalf("listing the findings: %v", err)
	}

	seen := map[ids.UUID]map[string]assurance.Exception{myDeal: {}, theirDeal: {}}
	for _, row := range found {
		if byType, ok := seen[row.SubjectID]; ok {
			byType[row.Type] = row
		}
	}
	for deal, byType := range seen {
		if len(byType) != 2 {
			t.Fatalf("deal %s listed %d findings, want both — the mask withholds figures, not rows", deal, len(byType))
		}
	}

	colleague := seen[theirDeal]
	for kind, row := range colleague {
		if row.AffectedMinor != nil {
			t.Errorf("%s on a colleague's deal listed affected_minor %d — deal.amount_minor is masked "+
				"outside the reader's write authority", kind, *row.AffectedMinor)
		}
	}
	if money := colleague[assurance.TypeAmountVsOffer]; money.Claim != nil || money.Observed != nil {
		t.Errorf("amount_vs_offer on a colleague's deal listed claim %s and observed %s — both "+
			"quote the masked amount", money.Claim, money.Observed)
	}
	if date := colleague[assurance.TypeClosePast]; !strings.Contains(string(date.Claim), "2026-04-30") {
		t.Errorf("close_past on a colleague's deal listed claim %s, want the date — the mask is on "+
			"the amount, and the date is not money", date.Claim)
	}

	// The control: the reader's own deal is theirs to change, so its figures read.
	own := seen[myDeal]
	if got := own[assurance.TypeAmountVsOffer].AffectedMinor; got == nil || *got != 200000 {
		t.Errorf("the reader's own amount_vs_offer listed affected_minor %v, want 200000", got)
	}
	if got := string(own[assurance.TypeAmountVsOffer].Claim); !strings.Contains(got, "500000") {
		t.Errorf("the reader's own amount_vs_offer listed claim %s, want the amount", got)
	}
}
