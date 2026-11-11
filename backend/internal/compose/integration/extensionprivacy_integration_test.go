// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The start-up check of an extension's privacy declaration, and the refusal to
// merge away a record an extension's rows depend on.

import (
	"context"
	"errors"
	"strings"
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
			contact_id uuid, company_id uuid, lead_id uuid, body text, noted_at timestamptz NOT NULL DEFAULT now())`,
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
	e.Contacts.WithExtensions(extprivacy.Declared([]privacy.Table{{
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

func TestBootRefusesAnEarlierVersionThatComesBackWithAnotherDeclaration(t *testing.T) {
	env := Setup(t)
	verify := func(e extension.Extension) error {
		return compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{e})
	}
	for _, step := range []extension.Extension{probeUnit(t, "1.0.0", "body"), probeUnit(t, "2.0.0")} {
		if err := verify(step); err != nil {
			t.Fatalf("boot of %s refused: %v", step.Version, err)
		}
	}
	changed := probeUnit(t, "1.0.0")
	changed.Privacy[0].Links = changed.Privacy[0].Links[:1]
	if err := verify(changed); err == nil {
		t.Error("1.0.0 came back with a declaration 1.0.0 never made and booted")
	}
	if err := verify(probeUnit(t, "1.0.0", "body")); err != nil {
		t.Errorf("a downgrade to 1.0.0 with the declaration 1.0.0 made was refused: %v", err)
	}
}

// probeExec runs owner statements a test needs on the probe table.
func probeExec(t *testing.T, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := OwnerConn(t).Exec(t.Context(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func TestBootRefusesALinkColumnThatIsNotAUUID(t *testing.T) {
	env := Setup(t)
	unit := probeUnit(t, "1.0.0")
	probeExec(t, `ALTER TABLE ext.ext_probe_note ADD COLUMN contact_ref text`)
	unit.Privacy[0].Links = []privacy.Link{{Column: "contact_ref", Record: privacy.Contact}}

	if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit}); err == nil {
		t.Error("a text link column booted; every lookup by record id would fail when it is due")
	}
}

func TestBootRefusesARequiredPersonalColumnABlankValueWouldBreak(t *testing.T) {
	const nick = `ALTER TABLE ext.ext_probe_note ADD COLUMN nick text NOT NULL DEFAULT 'x'`
	for name, statements := range map[string][]string{
		"check":             {nick + ` CHECK (length(nick) > 0)`},
		"unique":            {nick + ` UNIQUE`},
		"expression unique": {nick, `CREATE UNIQUE INDEX ext_probe_note_nick_idx ON ext.ext_probe_note (lower(nick))`},
		"foreign key": {
			`CREATE TABLE ext.ext_probe_nick (nick text PRIMARY KEY)`,
			`INSERT INTO ext.ext_probe_nick VALUES ('x')`,
			nick + ` REFERENCES ext.ext_probe_nick (nick)`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := Setup(t)
			unit := probeUnit(t, "1.0.0")
			t.Cleanup(func() {
				if _, err := OwnerConn(t).Exec(context.Background(), `DROP TABLE IF EXISTS ext.ext_probe_nick CASCADE`); err != nil {
					t.Errorf("removing the referenced table: %v", err)
				}
			})
			probeExec(t, statements...)
			unit.Privacy[0].Personal = []string{"nick"}

			if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit}); err == nil {
				t.Error("a required personal column with a constraint the blank value breaks booted")
			}
		})
	}
}

func TestBootLeavesAUnitTheTablesOfALongerNamedUnit(t *testing.T) {
	env := Setup(t)
	probe := probeUnit(t, "1.0.0", "body")
	probeExec(t,
		`CREATE TABLE ext.ext_probe_extra_note (id uuid PRIMARY KEY, contact_id uuid)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_probe_extra_note TO margince_app`)
	t.Cleanup(func() {
		if _, err := OwnerConn(t).Exec(context.Background(), `DROP TABLE IF EXISTS ext.ext_probe_extra_note`); err != nil {
			t.Errorf("removing the second unit's table: %v", err)
		}
	})
	extra := extension.Extension{Name: "probe-extra", Version: "1.0.0", Privacy: []privacy.Table{{
		Name:      "ext_probe_extra_note",
		Links:     []privacy.Link{{Column: "contact_id", Record: privacy.Contact}},
		Retention: jurisdiction.CommercialCorrespondence,
	}}}

	if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{probe, extra}); err != nil {
		t.Errorf("unit probe was held to the tables of unit probe-extra: %v", err)
	}
}

func TestBootRefusesAPersonalTableWhoseRowsTheCoreCannotAuditByID(t *testing.T) {
	for name, table := range map[string]string{
		"no id column":  `CREATE TABLE ext.ext_probe_log (contact_id uuid, body text)`,
		"id not uuid":   `CREATE TABLE ext.ext_probe_log (id bigint PRIMARY KEY, contact_id uuid, body text)`,
		"id not unique": `CREATE TABLE ext.ext_probe_log (id uuid NOT NULL, contact_id uuid, body text)`,
		"id nullable":   `CREATE TABLE ext.ext_probe_log (id uuid UNIQUE, contact_id uuid, body text)`,
		"id under an invalid unique index": `CREATE TABLE ext.ext_probe_log (id uuid NOT NULL, contact_id uuid, body text);
			CREATE UNIQUE INDEX ext_probe_log_id ON ext.ext_probe_log (id);
			UPDATE pg_index SET indisvalid = false WHERE indexrelid = 'ext.ext_probe_log_id'::regclass`,
	} {
		t.Run(name, func(t *testing.T) {
			env := Setup(t)
			unit := probeUnit(t, "1.0.0")
			t.Cleanup(func() {
				if _, err := OwnerConn(t).Exec(context.Background(), `DROP TABLE IF EXISTS ext.ext_probe_log`); err != nil {
					t.Errorf("removing the probe log: %v", err)
				}
			})
			probeExec(t, table, `GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_probe_log TO margince_app`)
			unit.Privacy = append(unit.Privacy, privacy.Table{
				Name:      "ext_probe_log",
				Links:     []privacy.Link{{Column: "contact_id", Record: privacy.Contact}},
				Personal:  []string{"body"},
				Retention: jurisdiction.CommercialCorrespondence,
			})

			err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit})
			if err == nil || !strings.Contains(err.Error(), "ext_probe_log.id") {
				t.Errorf("boot answered %v; want a refusal naming ext_probe_log.id", err)
			}
		})
	}
}

func TestBootRefusesALinkColumnTheTableLacks(t *testing.T) {
	env := Setup(t)
	unit := probeUnit(t, "1.0.0")
	unit.Privacy[0].Links = []privacy.Link{{Column: "deal_ref", Record: privacy.Contact}}

	err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit})
	if err == nil || !strings.Contains(err.Error(), "ext_probe_note.deal_ref") {
		t.Errorf("boot answered %v; want a refusal naming the missing link column", err)
	}
}

func TestBootRefusesAnAgeColumnARetentionWindowCannotCountFrom(t *testing.T) {
	for name, column := range map[string]struct{ name, add string }{
		"missing":             {name: "kept_at"},
		"not a point in time": {name: "body"},
		"nullable":            {name: "aged_at", add: `aged_at timestamptz`},
		"without a time zone": {name: "aged_at", add: `aged_at timestamp NOT NULL DEFAULT now()`},
	} {
		t.Run(name, func(t *testing.T) {
			env := Setup(t)
			unit := probeUnit(t, "1.0.0")
			if column.add != "" {
				probeExec(t, `ALTER TABLE ext.ext_probe_note ADD COLUMN `+column.add)
			}
			unit.Privacy[0].AgeColumn = column.name

			err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit})
			if err == nil || !strings.Contains(err.Error(), "age column") {
				t.Errorf("age column %q booted (err=%v); a retention sweep could not count every row's age from it", column.name, err)
			}
		})
	}
}

func TestBootAcceptsADeclaredAgeColumn(t *testing.T) {
	for name, column := range map[string]string{"timestamptz": "noted_at", "date": "noted_on"} {
		t.Run(name, func(t *testing.T) {
			env := Setup(t)
			unit := probeUnit(t, "1.0.0", "body")
			probeExec(t, `ALTER TABLE ext.ext_probe_note ADD COLUMN noted_on date NOT NULL DEFAULT current_date`)
			unit.Privacy[0].AgeColumn = column

			if err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit}); err != nil {
				t.Errorf("a %s age column was refused: %v", name, err)
			}
		})
	}
}

func TestBootFollowsACascadeToTheReferenceThatWouldStopIt(t *testing.T) {
	for clause, refused := range map[string]bool{"ON DELETE RESTRICT": true, "ON DELETE CASCADE": false} {
		t.Run(clause, func(t *testing.T) {
			env := Setup(t)
			unit := probeUnit(t, "1.0.0", "body")
			unit.Privacy[0].AgeColumn = "noted_at"
			t.Cleanup(func() {
				if _, err := OwnerConn(t).Exec(context.Background(), `DROP TABLE IF EXISTS ext.ext_probe_quote, ext.ext_probe_reply`); err != nil {
					t.Errorf("removing the referencing tables: %v", err)
				}
			})
			probeExec(t,
				`CREATE TABLE ext.ext_probe_reply (id uuid PRIMARY KEY, note_id uuid REFERENCES ext.ext_probe_note (id) ON DELETE CASCADE)`,
				`CREATE TABLE ext.ext_probe_quote (id uuid PRIMARY KEY, reply_id uuid REFERENCES ext.ext_probe_reply (id) `+clause+`)`,
				`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_probe_reply, ext.ext_probe_quote TO margince_app`)
			unit.Privacy = append(unit.Privacy,
				privacy.Table{Name: "ext_probe_reply", Retention: jurisdiction.CommercialCorrespondence},
				privacy.Table{Name: "ext_probe_quote", Retention: jurisdiction.CommercialCorrespondence})

			err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit})
			if refused && (err == nil || !strings.Contains(err.Error(), "ext_probe_quote") || !strings.Contains(err.Error(), "ext_probe_reply")) {
				t.Errorf("boot answered %v; want a refusal that names the cascade path to the blocking reference", err)
			}
			if !refused && err != nil {
				t.Errorf("boot refused an all-cascade chain: %v", err)
			}
		})
	}
}

func TestBootLetsOnlyACascadeReferenceASweptTable(t *testing.T) {
	for clause, refused := range map[string]bool{
		"": true, "ON DELETE RESTRICT": true, "ON DELETE SET NULL": true, "ON DELETE SET DEFAULT": true,
		"ON DELETE CASCADE": false,
	} {
		t.Run(clause, func(t *testing.T) {
			column := "note_id uuid REFERENCES ext.ext_probe_note (id) " + clause
			env := Setup(t)
			unit := probeUnit(t, "1.0.0", "body")
			unit.Privacy[0].AgeColumn = "noted_at"
			t.Cleanup(func() {
				if _, err := OwnerConn(t).Exec(context.Background(), `DROP TABLE IF EXISTS ext.ext_probe_reply`); err != nil {
					t.Errorf("removing the referencing table: %v", err)
				}
			})
			probeExec(t, `CREATE TABLE ext.ext_probe_reply (id uuid PRIMARY KEY, `+column+`)`,
				`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_probe_reply TO margince_app`)
			unit.Privacy = append(unit.Privacy, privacy.Table{Name: "ext_probe_reply", Retention: jurisdiction.CommercialCorrespondence})

			err := compose.VerifyExtensionPrivacy(t.Context(), env.Pool, []extension.Extension{unit})
			if refused && (err == nil || !strings.Contains(err.Error(), "ext_probe_reply") || !strings.Contains(err.Error(), "ON DELETE CASCADE")) {
				t.Errorf("boot answered %v; want a refusal that names the referencing table and asks for ON DELETE CASCADE", err)
			}
			if !refused && err != nil {
				t.Errorf("boot refused a cascading reference: %v", err)
			}
		})
	}
}
