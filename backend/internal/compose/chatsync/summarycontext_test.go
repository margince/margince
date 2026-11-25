// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// groupAnswers stands in for the database: each statement a group of readers
// sends is answered with the next of its rows. A company name of "" is no row.
type groupAnswers struct {
	pgx.Tx
	stakeholders [][]stakeholder
	companies    []string
	statements   int
}

//craft:ignore naked-any pgx passes a statement's arguments as any
func (g *groupAnswers) Query(context.Context, string, ...any) (pgx.Rows, error) {
	g.statements++
	return &stakeholderRows{rows: g.stakeholders[g.statements-1], at: -1}, nil
}

//craft:ignore naked-any pgx passes a statement's arguments as any
func (g *groupAnswers) QueryRow(context.Context, string, ...any) pgx.Row {
	g.statements++
	return companyRow(g.companies[g.statements-1])
}

type companyRow string

//craft:ignore naked-any pgx scans into whatever its caller points at
func (r companyRow) Scan(dest ...any) error {
	if r == "" {
		return pgx.ErrNoRows
	}
	name, ok := dest[0].(*string)
	if !ok {
		return pgx.ErrNoRows
	}
	*name = string(r)
	return nil
}

type stakeholderRows struct {
	pgx.Rows
	rows []stakeholder
	at   int
}

func (r *stakeholderRows) Next() bool { r.at++; return r.at < len(r.rows) }
func (r *stakeholderRows) Err() error { return nil }
func (r *stakeholderRows) Close()     {}

//craft:ignore naked-any pgx scans into whatever its caller points at
func (r *stakeholderRows) Scan(dest ...any) error {
	id, idOK := dest[0].(*ids.UUID)
	name, nameOK := dest[1].(*string)
	if !idOK || !nameOK {
		return pgx.ErrNoRows
	}
	*id, *name = r.rows[r.at].id, r.rows[r.at].name
	return nil
}

// twoGroupsOfReaders is one reader more than a statement judges, so the
// readers take two statements.
func twoGroupsOfReaders() (context.Context, []principal.Principal) {
	readers := make([]principal.Principal, auth.MaxReadersPerClause+1)
	for i := range readers {
		id := ids.NewV7()
		readers[i] = principal.Principal{
			Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + id.String(), UserID: id,
			Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
				"company": {Read: true}, "contact": {Read: true}, "relationship": {Read: true},
			}},
		}
	}
	return principal.WithActor(context.Background(), chatLinksPrincipal), readers
}

func TestAStakeholderIsNamedOnceAndOnlyWhenEveryGroupSeesIt(t *testing.T) {
	ctx, readers := twoGroupsOfReaders()
	clara, olaf := stakeholder{ids.NewV7(), "Clara Buyer"}, stakeholder{ids.NewV7(), "Olaf Other"}
	db := &groupAnswers{stakeholders: [][]stakeholder{{clara, olaf, clara}, {clara}}}
	names, err := stakeholderNames(ctx, db, ids.NewV7(), readers)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Clara Buyer"}; !slices.Equal(names, want) {
		t.Errorf("names = %q, want %q: Olaf is not seen by the second group, and Clara has two edges", names, want)
	}
}

func TestNoGroupIsAskedOnceTheFirstSeesNoStakeholder(t *testing.T) {
	ctx, readers := twoGroupsOfReaders()
	db := &groupAnswers{stakeholders: [][]stakeholder{{}, {{ids.NewV7(), "Clara Buyer"}}}}
	names, err := stakeholderNames(ctx, db, ids.NewV7(), readers)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 || db.statements != 1 {
		t.Errorf("names = %q after %d statements, want none after the first", names, db.statements)
	}
}

func TestTheCompanyIsLeftOutWhenALaterGroupDoesNotSeeIt(t *testing.T) {
	ctx, readers := twoGroupsOfReaders()
	db := &groupAnswers{companies: []string{"Acme GmbH", ""}}
	name, err := companyName(ctx, db, ids.NewV7(), readers)
	if err != nil {
		t.Fatal(err)
	}
	if name != "" {
		t.Errorf("company = %q, want it left out: the second group sees no row", name)
	}
}
