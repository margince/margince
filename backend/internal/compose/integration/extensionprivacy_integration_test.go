// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The start-up check of an extension's privacy declaration, and the refusal to
// merge away a record an extension's rows depend on.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// probeUnit creates `ext_probe_note` as a unit's migration would and returns
// the unit declaring it at the given version.
func probeUnit(t *testing.T, version extension.Version, personal ...string) extension.Extension {
	t.Helper()
	owner := OwnerConn(t)
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`CREATE TABLE IF NOT EXISTS ext.ext_probe_note (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			contact_id uuid, company_id uuid, lead_id uuid, body text,
			created_at timestamptz NOT NULL DEFAULT now())`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_probe_note TO margince_app`,
	} {
		if _, err := owner.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("preparing the probe table (%s): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.ext_probe_note`); err != nil {
			t.Errorf("removing the probe table: %v", err)
		}
	})
	return extension.Extension{
		Name: "probe", Version: version,
		Privacy: []privacy.Table{{
			Name:      "ext_probe_note",
			Links:     []privacy.Link{{Column: "contact_id", Record: privacy.Contact}, {Column: "company_id", Record: privacy.Company}, {Column: "lead_id", Record: privacy.Lead}},
			Personal:  personal,
			Retention: jurisdiction.CommercialCorrespondence,
		}},
	}
}

func TestBootRefusesAChangedDeclarationWithoutANewVersion(t *testing.T) {
	env := Setup(t)
	verify := func(e extension.Extension) error {
		return compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{e})
	}

	if err := verify(probeUnit(t, "1.0.0", "body")); err != nil {
		t.Fatalf("first boot refused a complete declaration: %v", err)
	}
	if err := verify(probeUnit(t, "1.0.0", "body")); err != nil {
		t.Fatalf("an unchanged reboot was refused: %v", err)
	}
	if err := verify(probeUnit(t, "1.0.0")); err == nil {
		t.Error("a declaration that dropped a personal column kept its version and booted")
	}
	if err := verify(probeUnit(t, "1.1.0")); err != nil {
		t.Errorf("the same change under a new version was refused: %v", err)
	}
}

func TestBootAcceptsAKeylessTableThatHoldsNoPersonalColumn(t *testing.T) {
	env := Setup(t)
	unit := probeUnit(t, "1.0.0", "body")
	owner := OwnerConn(t)
	for _, stmt := range []string{
		`CREATE TABLE ext.ext_probe_marker (activity_id uuid PRIMARY KEY)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_probe_marker TO margince_app`,
	} {
		if _, err := owner.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.ext_probe_marker`); err != nil {
			t.Errorf("removing the marker table: %v", err)
		}
	})
	unit.Privacy = append(unit.Privacy, privacy.Table{
		Name:      "ext_probe_marker",
		Links:     []privacy.Link{{Column: "activity_id", Record: privacy.Activity}},
		Retention: jurisdiction.CommercialCorrespondence,
	})

	if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit}); err != nil {
		t.Errorf("a table with a link, no id and no personal column was refused: %v", err)
	}
}

func TestBootRefusesATableTheDeclarationLeavesOut(t *testing.T) {
	env := Setup(t)
	unit := probeUnit(t, "1.0.0", "body")
	unit.Privacy = nil

	if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit}); err == nil {
		t.Error("a unit table with no declaration booted")
	}
}

func TestBootRefusesADeclaredColumnTheTableLacks(t *testing.T) {
	env := Setup(t)

	if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{probeUnit(t, "1.0.0", "no_such_column")}); err == nil {
		t.Error("a declared personal column the table does not have booted")
	}
}

func TestBootChecksTheAgeColumnTheSweepCountsFrom(t *testing.T) {
	env := Setup(t)
	for i, c := range []struct {
		column   string
		wantBoot bool
	}{{"created_at", true}, {"no_such_column", false}, {"body", false}} {
		column, wantBoot := c.column, c.wantBoot
		// A new version each time: the same version may not change its declaration.
		unit := probeUnit(t, extension.Version(fmt.Sprintf("1.%d.0", i)), "body")
		unit.Privacy[0].AgeColumn = column
		if column == "body" {
			// A personal column cannot be the age, so the type check is reached
			// through a table whose text column is not declared personal.
			unit.Privacy[0].Personal = nil
		}
		err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit})
		if wantBoot && err != nil {
			t.Errorf("age column %q was refused: %v", column, err)
		}
		if !wantBoot && err == nil {
			t.Errorf("age column %q booted, though the sweep cannot count a row's age from it", column)
		}
	}
}

func TestBootRefusesARequiredPersonalColumnTheCoreCannotBlank(t *testing.T) {
	env := Setup(t)
	unit := probeUnit(t, "1.0.0")
	if _, err := OwnerConn(t).Exec(t.Context(),
		`ALTER TABLE ext.ext_probe_note ADD COLUMN age integer NOT NULL DEFAULT 0`); err != nil {
		t.Fatal(err)
	}
	unit.Privacy[0].Personal = []string{"age"}

	if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit}); err == nil {
		t.Error("a required integer personal column booted, and the erasure would fail when it is due")
	}
}

func TestMergeIsRefusedWhileExtensionRecordsDependOnTheSource(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	probeUnit(t, "1.0.0")
	e.Contacts.WithExtensions(extprivacy.New([]privacy.Table{{
		Name:      "ext_probe_note",
		Links:     []privacy.Link{{Column: "contact_id", Record: privacy.Contact}},
		Retention: jurisdiction.CommercialCorrespondence,
	}}))

	source, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{FullName: "Source", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{FullName: "Target", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := OwnerConn(t).Exec(t.Context(),
			`INSERT INTO ext.ext_probe_note (contact_id) VALUES ($1)`, source.Id); err != nil {
			t.Fatal(err)
		}
	}

	_, err = e.Contacts.MergeContact(admin, ids.From[ids.ContactKind](ids.UUID(source.Id)), ids.From[ids.ContactKind](ids.UUID(target.Id)), nil)
	var refusal *contacts.DependentRecordsError
	if !errors.As(err, &refusal) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("merge of a depended-on contact = %v, want a conflict naming its dependents", err)
	}
	if refusal.Count != 2 {
		t.Errorf("the refusal counts %d dependents, want 2", refusal.Count)
	}
}
