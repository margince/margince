// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

// The seven named tenancy breaks have a test each next door, because each is
// worth reading on its own. The allowlist has more edges than those seven, and
// every one of them is a way to keep a tenant table's rows reachable from
// outside its workspace — so they are all provoked here, table-driven, one row
// per edge.
//
// A refusal with no test behind it is a refusal nobody has ever seen fire.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// refusal is one violation: the SQL that provokes it and the substrings the
// message must carry for an author to be able to act on it.
type refusal struct {
	tag         string // feeds the unit name, so each case gets its own role
	up          func(ns string) string
	down        func(ns string) string
	mustMention []string
}

func dropNote(ns string) string { return scaffoldDown(ns) }

func TestGateRefusesEveryShapeOutsideTheAllowlist(t *testing.T) {
	for _, c := range refusalCases() {
		t.Run(c.tag, func(t *testing.T) {
			unit := unitName(t, c.tag)
			ns := namespaceOf(t, unit)
			err := runGate(t, unit, migrationDir(t, c.up(ns), c.down(ns)))
			requireRefusal(t, err, c.mustMention...)
		})
	}
}

// valid is the shape the gate accepts; the grant and object rows below add one
// thing to it, and the policy rows build their own table instead.
func valid(ns string) string { return scaffoldUp(ns) }

// policyBody is an ordinary, correct-looking predicate over a column the table
// actually has. The rule is that a unit table carries no policy AT ALL, so the
// row proving it must not be refusable for anything the body itself says.
var policyBody = "USING (created_at IS NOT NULL) WITH CHECK (created_at IS NOT NULL)"

// refusalCases is split into three groups only because the craft gate caps a
// function's length; the grouping follows what each row is about.
func refusalCases() []refusal {
	cases := grantRefusals()
	cases = append(cases, policyRefusals()...)
	return append(cases, relationRefusals()...)
}

// grantRefusals: who may hold a privilege on an extension table, and which.
func grantRefusals() []refusal {
	return []refusal{{
		tag:  "grantpub",
		up:   func(ns string) string { return valid(ns) + fmt.Sprintf("GRANT SELECT ON ext.%s_note TO PUBLIC;\n", ns) },
		down: dropNote,
		// PUBLIC includes roles created after the unit was installed, which no
		// review of the unit's own SQL can enumerate.
		mustMention: []string{"_note", "to PUBLIC"},
	}, {
		tag: "granttrunc",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf("GRANT TRUNCATE ON ext.%s_note TO margince_app;\n", ns)
		},
		down:        dropNote,
		mustMention: []string{"margince_app", "arwdD"},
	}, {
		tag: "grantother",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf("GRANT SELECT ON ext.%s_note TO margince_owner;\n", ns)
		},
		down:        dropNote,
		mustMention: []string{"margince_owner", "only the owner and margince_app"},
	}, {
		tag: "grantcol",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf("GRANT UPDATE (body) ON ext.%s_note TO margince_app;\n", ns)
		},
		down:        dropNote,
		mustMention: []string{"column-level grant", "body"},
	}, {
		// The other side of the allowlist, and it was missing: "nothing outside
		// the list" is satisfied perfectly by granting NOTHING, and the table
		// then answers `permission denied` at the first handler call.
		tag: "grantnone",
		up: func(ns string) string {
			return strings.Replace(valid(ns),
				fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON ext.%s_note TO margince_app;\n", ns), "", 1)
		},
		down:        dropNote,
		mustMention: []string{"margince_app", "must grant exactly"},
	}, {
		tag: "grantpartial",
		up: func(ns string) string {
			return strings.Replace(valid(ns),
				fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON ext.%s_note TO margince_app;\n", ns),
				fmt.Sprintf("GRANT SELECT ON ext.%s_note TO margince_app;\n", ns), 1)
		},
		down:        dropNote,
		mustMention: []string{`grants "r"`, "must grant exactly"},
	}}
}

// policyRefusals: the single policy's dimensions, and the tenant column the
// policy compares.
func policyRefusals() []refusal {
	return []refusal{{
		// A workspace column, the shape the tier carried until an installation
		// became single-organization. Refused rather than tolerated: it is the
		// column a policy would key on, and its presence is what makes the next
		// author believe there is a tenant wall here.
		tag: "tenantcolumn",
		up: func(ns string) string {
			return noteTable(ns, "", tenantColumnSQL) + noteGrant(ns)
		},
		down:        dropNote,
		mustMention: []string{"workspace_id", "one workspace"},
	}, {
		// RLS on with no policy: every row denied to a non-owner, and a table
		// that reads at a glance as one somebody protected.
		tag: "rlsnopolicy",
		up: func(ns string) string {
			return noteTable(ns, "", "") + noteRLS(ns) + noteGrant(ns)
		},
		down:        dropNote,
		mustMention: []string{"relrowsecurity=true", "no row-level rule"},
	}, {
		// A policy, whatever it says. The predicate below is the one the tier
		// used to require, so this row proves the rule is "no policy" and not
		// "no BADLY WRITTEN policy".
		tag: "policy",
		up: func(ns string) string {
			return noteTable(ns, "", "") + noteRLS(ns) +
				notePolicy(ns, policyBody) + noteGrant(ns)
		},
		down:        dropNote,
		mustMention: []string{"row-level"},
	}, {
		// A policy with RLS left off is dead text PostgreSQL never consults,
		// and the reading it invites — that the table is isolated — is exactly
		// the one a later author would carry forward.
		tag: "deadpolicy",
		up: func(ns string) string {
			// No ALTER at all: PostgreSQL accepts CREATE POLICY on a table with
			// row-level security switched off, and never consults it.
			return noteTable(ns, "", "") + notePolicy(ns, policyBody) + noteGrant(ns)
		},
		down:        dropNote,
		mustMention: []string{"policy", "dead text"},
	}, {
		// A foreign key into core. The role holds nothing on public, so this is
		// refused inside the apply rather than by a catalog assertion — which is
		// the stronger place for it, and the reason the grant was dropped.
		tag: "corefk",
		up: func(ns string) string {
			return noteTable(ns, "", "owner_id uuid NOT NULL REFERENCES workspace(id)") + noteGrant(ns)
		},
		down:        dropNote,
		mustMention: []string{"permission denied"},
	}}
}

// relationRefusals: what may exist in the ext schema at all, and what must be
// gone after the revert.
func relationRefusals() []refusal {
	return []refusal{{
		tag: "unlogged",
		up: func(ns string) string {
			return noteTable(ns, "UNLOGGED ", tenantColumnSQL) +
				noteRLS(ns) + notePolicy(ns, policyBody) + noteGrant(ns)
		},
		down:        dropNote,
		mustMention: []string{"relpersistence", "loses its rows"},
	}, {
		tag: "view",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf("CREATE VIEW ext.%[1]s_all AS SELECT * FROM ext.%[1]s_note;\n", ns)
		},
		down: func(ns string) string {
			return fmt.Sprintf("DROP VIEW IF EXISTS ext.%s_all;\n", ns) + scaffoldDown(ns)
		},
		mustMention: []string{"is a VIEW", "owner's rights"},
	}, {
		tag: "routine",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf("CREATE FUNCTION ext.%s_count() RETURNS bigint LANGUAGE sql AS 'SELECT 1::bigint';\n", ns)
		},
		down: func(ns string) string {
			return fmt.Sprintf("DROP FUNCTION IF EXISTS ext.%s_count();\n", ns) + scaffoldDown(ns)
		},
		mustMention: []string{"routine ext.", "_count", "outside the ext schema's table allowlist"},
	}, {
		tag: "enumtype",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf("CREATE TYPE ext.%s_kind AS ENUM ('a', 'b');\n", ns)
		},
		down: func(ns string) string {
			return fmt.Sprintf("DROP TYPE IF EXISTS ext.%s_kind;\n", ns) + scaffoldDown(ns)
		},
		mustMention: []string{"type ext.", "_kind"},
	}, {
		tag: "defaultacl",
		up: func(ns string) string {
			return valid(ns) + "ALTER DEFAULT PRIVILEGES IN SCHEMA ext GRANT SELECT ON TABLES TO PUBLIC;\n"
		},
		down: func(ns string) string {
			return "ALTER DEFAULT PRIVILEGES IN SCHEMA ext REVOKE SELECT ON TABLES FROM PUBLIC;\n" + scaffoldDown(ns)
		},
		mustMention: []string{"default privileges", "objects that do not exist yet"},
	}, {
		// suppress_redundant_updates_trigger is a built-in owned by the
		// bootstrap superuser, so this provokes the trigger rule WITHOUT also
		// tripping the routine rule, which runs first.
		tag: "trigger",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf(
				"CREATE TRIGGER %[1]s_note_squash BEFORE UPDATE ON ext.%[1]s_note FOR EACH ROW EXECUTE FUNCTION suppress_redundant_updates_trigger();\n", ns)
		},
		down:        dropNote,
		mustMention: []string{"_note_squash", "arbitrary code"},
	}, {
		// A rule is the write-path hook pg_trigger does not cover, and the role
		// owns its own tables so it needs no further privilege to install one.
		tag: "rule",
		up: func(ns string) string {
			return valid(ns) + fmt.Sprintf("CREATE RULE %[1]s_note_swallow AS ON INSERT TO ext.%[1]s_note DO INSTEAD NOTHING;\n", ns)
		},
		down:        dropNote,
		mustMention: []string{"_note_swallow", "DO INSTEAD NOTHING discards them entirely"},
	}, {
		// A sequence is a relation the namespace check alone used to clear.
		tag: "seqgrant",
		up: func(ns string) string {
			return valid(ns) +
				fmt.Sprintf("CREATE SEQUENCE ext.%[1]s_note_no;\nGRANT ALL ON SEQUENCE ext.%[1]s_note_no TO PUBLIC;\n", ns)
		},
		down: func(ns string) string {
			return fmt.Sprintf("DROP SEQUENCE IF EXISTS ext.%s_note_no;\n", ns) + scaffoldDown(ns)
		},
		mustMention: []string{"_note_no", "to PUBLIC"},
	}, {
		// The down half is syntactically fine and reverts nothing.
		tag:         "partialdown",
		up:          valid,
		down:        func(string) string { return "-- deliberately reverts nothing\n" },
		mustMention: []string{"the down-migrations left these behind", "relation ext."},
	}, {
		// gen-composition's textual rule accepts an UNQUALIFIED create and
		// documents that it fails at apply time because the role holds CREATE
		// on ext alone. This is that claim, exercised.
		tag: "unqualified",
		up: func(ns string) string {
			return fmt.Sprintf("CREATE TABLE %s_note (id uuid NOT NULL PRIMARY KEY);\n", ns)
		},
		down:        func(ns string) string { return fmt.Sprintf("DROP TABLE IF EXISTS %s_note;\n", ns) },
		mustMention: []string{"permission denied for schema public", "0001_gate.up.sql"},
	}}
}

// The gate is also a command, and a command's arguments are part of its
// contract: a missing one must say which, not fail somewhere inside pgx.
func TestGateRefusesIncompleteInvocations(t *testing.T) {
	ctx := context.Background()
	dir := migrationDir(t, scaffoldUp("ext_x"), scaffoldDown("ext_x"))
	dsn := ownerDSN(t)

	for _, c := range []struct {
		name           string
		unit, dir, dsn string
		mustMention    string
	}{
		{"no unit", "", dir, dsn, "-unit is required"},
		{"no dir", "u", "", dsn, "-dir is required"},
		{"no dsn", "u", dir, "", "-dsn is required"},
		{"invalid unit name", "Not A Unit", dir, dsn, "not a valid unit name"},
		{"missing directory", "u", filepath.Join(dir, "nope"), dsn, "reading nope"},
		{"unreachable database", "u", dir, "postgres://nobody@127.0.0.1:1/none", "connecting to the throwaway database"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := run(ctx, c.unit, c.dir, c.dsn)
			requireRefusal(t, err, c.mustMention)
		})
	}
}

// An empty migrations directory is a unit that declared the layer and then
// shipped nothing in it — which reads, everywhere else in the build, as a unit
// with migrations.
func TestGateRefusesAnEmptyMigrationsDirectory(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "migrations")
	if err := os.Mkdir(empty, 0o750); err != nil {
		t.Fatalf("creating the empty migrations directory: %v", err)
	}
	err := run(context.Background(), unitName(t, "empty"), empty, ownerDSN(t))
	requireRefusal(t, err, "holds no NNNN_name.up.sql/.down.sql pair")
}

// The gate refuses to share its role with a concurrent run rather than
// adopting or dropping it: the role is cluster-scoped and named from the unit,
// so dropping one in use would corrupt the OTHER run.
func TestGateRefusesWhileAnotherRunHoldsTheRole(t *testing.T) {
	unit := unitName(t, "busy")
	ns := namespaceOf(t, unit)
	owner := admin(t)
	ctx := context.Background()

	password, err := randomPassword()
	if err != nil {
		t.Fatalf("minting the stand-in role's password: %v", err)
	}
	if _, err := owner.Exec(ctx, `CREATE ROLE `+ns+` LOGIN PASSWORD '`+password+`' NOSUPERUSER NOBYPASSRLS`); err != nil {
		t.Fatalf("creating the stand-in role: %v", err)
	}
	t.Cleanup(func() {
		for _, statement := range []string{`DROP OWNED BY ` + ns + ` CASCADE`, `DROP ROLE IF EXISTS ` + ns} {
			if _, err := owner.Exec(context.Background(), statement); err != nil {
				t.Errorf("removing the stand-in role (%s): %v", statement, err)
			}
		}
	})

	held := connectAs(t, ns, password)
	defer func() {
		if err := held.Close(context.Background()); err != nil {
			t.Errorf("closing the held session: %v", err)
		}
	}()

	requireRefusal(t, run(ctx, unit, migrationDir(t, scaffoldUp(ns), scaffoldDown(ns)), ownerDSN(t)),
		"live session", ns)
}

// The gate's whole strength is the privileges its role does NOT hold, so it
// checks them after connecting rather than assuming the CREATE ROLE it just
// issued produced what it asked for. These two provoke that check by changing
// what the cluster grants to PUBLIC underneath it — which is exactly how a
// real cluster would drift into making this gate vacuous.
func TestGateRefusesARoleTheClusterHasWidened(t *testing.T) {
	owner := admin(t)
	ctx := context.Background()

	for _, c := range []struct {
		tag, name, setup, restore, mustMention string
	}{{
		tag:         "widenc",
		name:        "create on public",
		setup:       `GRANT CREATE ON SCHEMA public TO PUBLIC`,
		restore:     `REVOKE CREATE ON SCHEMA public FROM PUBLIC`,
		mustMention: "holds CREATE on schema public",
	}, {
		tag:         "widenu",
		name:        "no usage on public",
		setup:       `REVOKE USAGE ON SCHEMA public FROM PUBLIC`,
		restore:     `GRANT USAGE ON SCHEMA public TO PUBLIC`,
		mustMention: "cannot USE schema public",
	}, {
		// The DML-on-core refusal is PostgreSQL denying the statement, and this
		// is the cluster change that would make that denial stop happening while
		// every other assertion still passed.
		tag:         "widentbl",
		name:        "insert on every core table",
		setup:       `GRANT INSERT ON ALL TABLES IN SCHEMA public TO PUBLIC`,
		restore:     `REVOKE INSERT ON ALL TABLES IN SCHEMA public FROM PUBLIC`,
		mustMention: "can read, write or trigger on public.",
	}, {
		// TRIGGER is a write verb wearing another name: a role holding it can
		// install a function of its own on a core table, and every core write
		// runs it from then on. Nothing else in this gate would notice.
		tag:         "widentrg",
		name:        "trigger on every core table",
		setup:       `GRANT TRIGGER ON ALL TABLES IN SCHEMA public TO PUBLIC`,
		restore:     `REVOKE TRIGGER ON ALL TABLES IN SCHEMA public FROM PUBLIC`,
		mustMention: "can read, write or trigger on public.",
	}, {
		// The gate grants REFERENCES on public.workspace(id) itself, so the
		// verb cannot simply be refused — it is narrowed to that one column.
		// A cluster that widened it anywhere else lets a unit hang a foreign
		// key off a core table, which takes a lock on core writes and can
		// refuse a core delete forever after.
		tag:         "widenref",
		name:        "references on every core table",
		setup:       `GRANT REFERENCES ON ALL TABLES IN SCHEMA public TO PUBLIC`,
		restore:     `REVOKE REFERENCES ON ALL TABLES IN SCHEMA public FROM PUBLIC`,
		mustMention: "can declare a foreign key against public.",
	}} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := owner.Exec(ctx, c.setup); err != nil {
				t.Fatalf("%s: %v", c.setup, err)
			}
			// Restored before the assertion runs, so a failing assertion cannot
			// leave the rest of the package running on a widened cluster.
			defer func() {
				if _, err := owner.Exec(context.Background(), c.restore); err != nil {
					t.Fatalf("%s: %v", c.restore, err)
				}
			}()

			unit := unitName(t, c.tag)
			ns := namespaceOf(t, unit)
			requireRefusal(t, runGate(t, unit, migrationDir(t, scaffoldUp(ns), scaffoldDown(ns))), c.mustMention)
		})
	}
}

// assertNoRules excludes the rule named _RETURN, because that rule IS a view and
// a view in ext is already refused by name with a better message. This pins the
// reason that exclusion cannot be abused, rather than leaving it as a fact about
// PostgreSQL internals somebody verified by hand once.
//
// There are two worlds and the gate refuses in both, so the test accepts either
// and says which fired:
//
//   - PostgreSQL 16, where this runs: the server refuses an ON SELECT rule on a
//     table outright. There is no way to attach _RETURN to anything but a view.
//   - The legacy conversion, if a server still supports it: attaching _RETURN to
//     a table is the exact mechanism that TURNS it into one, so relkind flips to
//     'v' and assertRelationAllowed refuses it by name.
//
// The table is created bare — no primary key, no foreign key — because the
// legacy conversion refuses a table carrying indexes or constraints, and a
// fixture that failed for THAT reason would prove nothing about _RETURN.
func TestGateRefusesAManualReturnRule(t *testing.T) {
	unit := unitName(t, "retrule")
	ns := namespaceOf(t, unit)
	up := scaffoldUp(ns) + fmt.Sprintf(`
CREATE TABLE ext.%[1]s_asview (id uuid, body text);
CREATE RULE "_RETURN" AS ON SELECT TO ext.%[1]s_asview
    DO INSTEAD SELECT id, body FROM ext.%[1]s_note;
`, ns)
	down := fmt.Sprintf("DROP TABLE IF EXISTS ext.%s_asview CASCADE;\n", ns) + scaffoldDown(ns)

	err := runGate(t, unit, migrationDir(t, up, down))
	if err == nil {
		t.Fatal("a manual _RETURN rule must not reach the catalog unrefused — it is the one rule assertNoRules skips")
	}
	switch {
	case strings.Contains(err.Error(), "cannot have ON SELECT rules"):
		t.Logf("refused by PostgreSQL: an ON SELECT rule cannot be attached to a table at all\n%v", err)
	case strings.Contains(err.Error(), "is a VIEW"):
		t.Logf("refused by the catalog gate: the conversion flipped relkind to 'v'\n%v", err)
	default:
		t.Errorf("the gate refused, but for neither of the two reasons that make the _RETURN exclusion safe:\n%v", err)
	}
}
