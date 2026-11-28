// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package reportfixture

// A registered count or breakdown meets the report engine. The kind's read
// right admits it. A field mask excludes rows and says so. The group floor
// withholds the smallest groups and the total. A multi-select custom field
// splits a core population without counting a record twice.

import (
	"context"
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const Table = "ext_reportdemo_visit"

type Fixture struct {
	E       *integration.Env
	Catalog *customfields.Service
	Field   crmcontracts.CustomField
}

func Prepare(t *testing.T, register func(*testing.T)) Fixture {
	t.Helper()
	e := integration.Setup(t)
	register(t)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `ALTER TABLE ext.`+Table+` ADD COLUMN owner_id uuid; CREATE INDEX ON ext.`+Table+`(owner_id)`); err != nil {
		t.Fatal(err)
	}
	kinds := []extension.RecordKindDeclaration{{Kind: "visit", DisplayName: "Visit", Table: Table, OwnerColumn: "owner_id", NaturalKey: []string{"code"}, Fields: []extension.RecordKindField{{Name: "code", Label: "Code"}, {Name: "site", Label: "Site", Reportable: true}}}}
	recordkinds.Register("reportdemo", kinds)
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), customfields.FieldSpec{Object: "deal", Label: "Visit", Type: customfields.TypeRecord, Options: []string{"ext:reportdemo:visit"}, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := catalog.Update(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), ids.UUID(field.Id), customfields.FieldPatch{Reportable: &yes}, nil); err != nil {
		t.Fatal(err)
	}
	pipeline, open, _ := integration.DealFixture(t, e)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["ext_reportdemo_visit"] = principal.ObjectGrant{Read: true}
	writer := e.As(e.AdminUser, nil, perms)
	store := deals.NewStore(e.DB(), installseam.Deals()).WithFieldCatalog(catalog)
	for _, part := range []struct {
		site      string
		owner     ids.UUID
		dealOwner ids.UUID
		count     int
		archive   bool
	}{
		{"north", e.Rep1, e.Rep1, 6, false}, {"south", e.Rep2, e.Rep1, 2, false}, {"north", e.Rep1, e.Rep2, 1, false}, {"north", e.Rep3, e.Rep3, 1, false}, {"archived", e.Rep1, e.Rep1, 1, true},
	} {
		id := ids.NewV7()
		if _, err := owner.Exec(context.Background(), `INSERT INTO ext.`+Table+`(id,code,site,owner_id) VALUES($1,$2,$3,$4)`, id, id.String(), part.site, part.owner); err != nil {
			t.Fatal(err)
		}
		for i := range part.count {
			dealOwner := ids.From[ids.UserKind](part.dealOwner)
			if _, err := store.CreateDeal(writer, deals.CreateDealInput{Name: fmt.Sprintf("Joined %s %d", id, i), Source: "manual", PipelineID: pipeline, StageID: open, OwnerID: &dealOwner, CustomFields: map[string]any{*field.ColumnName: id.String()}}); err != nil {
				t.Fatal(err)
			}
		}
		if part.archive {
			if _, err := owner.Exec(context.Background(), `UPDATE ext.`+Table+` SET archived_at=now() WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	return Fixture{E: e, Catalog: catalog, Field: field}
}

func SeedVisits(t *testing.T, site string, live, archived int) {
	t.Helper()
	owner := integration.OwnerConn(t)
	for i := range live + archived {
		var archivedAt any
		if i >= live {
			archivedAt = "now()"
		}
		if _, err := owner.Exec(context.Background(),
			`INSERT INTO ext.`+Table+` (code, site, archived_at) VALUES ($1, $2, CASE WHEN $3::text IS NULL THEN NULL ELSE now() END)`,
			fmt.Sprintf("%s-%d", site, i), site, archivedAt); err != nil {
			t.Fatalf("seeding a visit: %v", err)
		}
	}
}

func Reader(e *integration.Env, mask ...principal.FieldMask) context.Context {
	return e.As(e.Rep1, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"ext_reportdemo_visit":  {Read: true},
			"installation_settings": {Read: true},
		},
		// Team scope, because a seat that reads every row is exempt from masks.
		RowScope:   principal.RowScopeTeam,
		FieldMasks: mask,
	})
}

func (f Fixture) Reader(scope principal.RowScope) context.Context {
	teams := []ids.UUID(nil)
	if scope == principal.RowScopeTeam {
		teams = []ids.UUID{f.E.Team1}
	}
	return f.E.As(f.E.Rep1, teams, principal.Permissions{Objects: map[string]principal.ObjectGrant{"deal": {Read: true}, "ext_reportdemo_visit": {Read: true}, "installation_settings": {Read: true}}, RowScope: scope})
}

func PrepareTable(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`DROP TABLE IF EXISTS ext.` + Table,
		`CREATE TABLE ext.` + Table + ` (id uuid PRIMARY KEY DEFAULT uuidv7(), archived_at timestamptz, code text NOT NULL UNIQUE, site text, tier integer NOT NULL DEFAULT 2)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + Table + ` TO margince_app`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the visit table (%s): %v", stmt, err)
		}
	}

}
