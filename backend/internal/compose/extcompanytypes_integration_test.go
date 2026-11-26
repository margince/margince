// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit's company types through the served entry point. Its own values land
// under its own name and another unit's stay. A name it never declared writes
// nothing.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/companytypes"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

// composeTypeUnits declares `notes` (the unit the fixture runs as) and a second
// unit that declares the same type name.
func composeTypeUnits(t *testing.T) {
	t.Helper()
	notes := extension.Extension{Name: coreUnit, CompanyTypes: []extension.CompanyType{
		{Type: "reseller", Label: "Reseller"}, {Type: "distributor", Label: "Distributor"},
	}}
	other := extension.Extension{Name: "other", CompanyTypes: []extension.CompanyType{{Type: "reseller", Label: "Other reseller"}}}
	setComposedExtensions([]extension.Extension{notes, other})
	companytypes.Reset()
	companytypes.Register(notes.Name, notes.CompanyTypes)
	companytypes.Register(other.Name, other.CompanyTypes)
	t.Cleanup(func() {
		setComposedExtensions(nil)
		companytypes.Reset()
	})
}

func (e coreEnv) seedCompany(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO company (id, owner_id, display_name, source, captured_by)
		VALUES ($1, $2, 'Typed Co', 'manual', 'human:x')`, id, e.user); err != nil {
		t.Fatalf("seeding the company: %v", err)
	}
	return id
}

func (e coreEnv) setTypes(t *testing.T, company ids.UUID, types []string) (crm.Company, error) {
	t.Helper()
	rt, ctx, cancel := e.runtime(t)
	defer cancel()
	var out crm.Company
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var inner error
		out, inner = tx.Core().Companies().SetExtensionTypes(ctx, company.String(), types)
		return inner
	})
	return out, err
}

func (e coreEnv) liveTypes(t *testing.T, company ids.UUID) []string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT relationship_type FROM company_relationship_type
		 WHERE company_id = $1 AND archived_at IS NULL ORDER BY relationship_type`, company)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func TestAUnitSetsItsDeclaredCompanyTypesUnderItsOwnName(t *testing.T) {
	e := setupCore(t)
	composeTypeUnits(t)
	company := e.seedCompany(t)

	got, err := e.setTypes(t, company, []string{"reseller"})
	if err != nil {
		t.Fatalf("SetExtensionTypes: %v", err)
	}
	want := []string{"ext:notes:reseller"}
	if got.ExtensionRelationshipTypes == nil || !slices.Equal(*got.ExtensionRelationshipTypes, want) {
		t.Errorf("answered types = %v, want %v", got.ExtensionRelationshipTypes, want)
	}
	if live := e.liveTypes(t, company); !slices.Equal(live, want) {
		t.Errorf("stored types = %v, want %v", live, want)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1 AND action = 'update'`, company); n != 1 {
		t.Errorf("%d audit row(s) for the edit, want 1", n)
	}
}

func TestAUnitReplacesOnlyItsOwnValuesAndKeepsAnotherUnits(t *testing.T) {
	e := setupCore(t)
	composeTypeUnits(t)
	company := e.seedCompany(t)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO company_relationship_type (company_id, relationship_type, source, captured_by)
		 VALUES ($1, 'ext:other:reseller', 'manual', 'human:x'), ($1, 'ext:notes:reseller', 'manual', 'human:x')`, company); err != nil {
		t.Fatal(err)
	}

	if _, err := e.setTypes(t, company, []string{"distributor"}); err != nil {
		t.Fatalf("SetExtensionTypes: %v", err)
	}
	if live, want := e.liveTypes(t, company), []string{"ext:notes:distributor", "ext:other:reseller"}; !slices.Equal(live, want) {
		t.Errorf("types = %v, want %v", live, want)
	}
	if _, err := e.setTypes(t, company, nil); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if live, want := e.liveTypes(t, company), []string{"ext:other:reseller"}; !slices.Equal(live, want) {
		t.Errorf("types after clearing the unit's own = %v, want %v", live, want)
	}
}

func TestAUnitCannotSetATypeItDidNotDeclare(t *testing.T) {
	e := setupCore(t)
	composeTypeUnits(t)
	company := e.seedCompany(t)

	for _, bad := range []string{"wholesaler", "ext:other:reseller", "ext:notes:reseller", ""} {
		if _, err := e.setTypes(t, company, []string{"reseller", bad}); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("type %q: err = %v, want ErrInvalid", bad, err)
		}
	}
	if live := e.liveTypes(t, company); len(live) != 0 {
		t.Errorf("a refused call wrote %v", live)
	}
}

func TestSettingTypesOnACompanyThatIsNotThereIsNotFound(t *testing.T) {
	e := setupCore(t)
	composeTypeUnits(t)

	if _, err := e.setTypes(t, ids.NewV7(), []string{"reseller"}); !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
