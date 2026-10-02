// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The who-is-waiting statement keys each reply-owner lookup on its link.
//
// As four plain joins the planner was free to treat the owner tables as a flat
// relation, and on production's data it did: underestimating the candidates
// ninefold, it rescanned all of contact and company once per candidate and
// spent 3.6 of the query's 4.1 seconds discarding 27 million rows in a join
// filter. The lookups are fenced laterals, and this holds that shape on the
// statement the store actually sends.

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingStatement keeps the who-is-waiting statement and its arguments as the
// store sent them, so the plan is of production's SQL rather than a copy. The
// store reads on the test's own goroutine, so it needs no lock.
type waitingStatement struct {
	sql  string
	args []any
}

func (w *waitingStatement) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "LEFT JOIN activity_participant sender") {
		w.sql, w.args = data.SQL, data.Args
	}
	return ctx
}

func (*waitingStatement) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestTheWaitingQueryKeysEachReplyOwnerLookupOnItsLink(t *testing.T) {
	e := setupLoad(t)
	sent := &waitingStatement{}
	config := e.pool.Config()
	config.ConnConfig.Tracer = sent
	pool, err := testdb.OwnPoolFromConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("opening a traced pool: %v", err)
	}
	defer pool.Close()
	store := NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	if _, err := store.WaitingReplies(e.as(), time.Now()); err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	if sent.sql == "" {
		t.Fatal("the who-is-waiting statement was never sent, so there is no plan to hold")
	}

	// Read off the STATEMENT, not the plan Postgres builds for it.
	//
	// The plan was the first spelling of this and it could not be made
	// deterministic: with every keyed-join mechanism disabled, what remains is a
	// nested loop, and where the link comparison lands inside it depends on costs
	// that are a coin flip at test volumes. It refused five pushes across two
	// branches while passing alone, and the alternatives do not work here — the
	// laterals already carry OFFSET 0, so flattening is not the variable, and
	// seeding enough rows to make keying plainly cheaper pollutes a database every
	// test in this package shares.
	//
	// What the statement says is what this product controls. The defect was four
	// plain joins; a fenced lateral keyed on the link is the fix, and a regression
	// to the old shape is a change to this text. That the planner then honours a
	// LATERAL … OFFSET 0 is Postgres's contract, not this product's to prove.
	for alias, column := range map[string]string{
		"ownerDeal": "wl.deal_id", "ownerLead": "wl.lead_id",
		"ownerContact": "wl.contact_id", "ownerCompany": "wl.company_id",
	} {
		keyed := regexp.MustCompile(
			`(?is)JOIN\s+LATERAL\s*\([^)]*\b` + regexp.QuoteMeta(alias) +
				`\.id\s*=\s*` + regexp.QuoteMeta(column) + `\b[^)]*OFFSET\s+0\s*\)`)
		if !keyed.MatchString(sent.sql) {
			t.Errorf("%s is not reached through a lateral keyed on %s and fenced with OFFSET 0: "+
				"as a plain join the planner is free to treat the owner tables as one flat "+
				"relation, which is what it did in production", alias, column)
		}
	}
}
