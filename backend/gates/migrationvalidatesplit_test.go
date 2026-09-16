// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A constraint is validated in a migration of its OWN, or the two-step buys
// nothing.
//
// `ADD CONSTRAINT … NOT VALID` then `VALIDATE CONSTRAINT` is written down in
// this tree as the safe way to constrain a large table: the first statement
// records the constraint without scanning, and the second checks the existing
// rows under a SHARE UPDATE EXCLUSIVE lock that readers and writers pass.
//
// That is true of the STATEMENTS and false of the migration, because
// dbmigrate.Up runs each file and its ledger row in ONE transaction
// (platform/dbmigrate/dbmigrate.go, inTx). The ACCESS EXCLUSIVE that ALTER took
// is held until that transaction commits, so a VALIDATE in the same file asks
// for a weaker lock while a stronger one is already held and nothing passes.
// The window the split exists to shorten is exactly as long as the one-step's.
//
// The tree worked this out for itself and wrote it down —
// `1787968163_the_brief_currency_check_is_validated.up.sql` states it almost
// word for word — and then seven more same-file migrations landed after it.
// That is what a gate is for: the correct shape was known and being copied over
// anyway, because the comment on the broken shape reads exactly as convincing.
//
// THE REGISTER IS CLOSED. The fourteen below are applied and immutable: a
// migration's content is digested into the ledger (dbmigrate.Digest), so
// editing one refuses every database that already ran it. They stay as they
// are; nothing new joins them.
//
// WHAT THIS CANNOT SEE: a validate landing in its own file but in the same PUSH
// as the add. That is still correct — two files are two transactions whenever
// they run — so there is nothing to catch.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The two statements, matched on the SQL rather than on the line: both appear
// in prose in these files — the comments explain the pattern at length — and a
// line-wise scan reads an explanation as an act.
var (
	addsNotValid     = regexp.MustCompile(`(?is)ADD\s+CONSTRAINT\b[^;]*?\bNOT\s+VALID\b`)
	validatesInPlace = regexp.MustCompile(`(?is)\bVALIDATE\s+CONSTRAINT\b`)
)

// sameTransactionValidators are the migrations that pair the two statements in
// one file. Applied and immutable, so this is a register that only shrinks —
// and it shrinks only if one is ever reverted, never by editing.
var sameTransactionValidators = map[string]bool{
	"1787352862_audit_admits_a_buyer_actor.up.sql":                       true,
	"1787357528_audit_admits_deal_room_lifecycle_verbs.up.sql":           true,
	"1787364347_audit_admits_deal_room_access_verbs.up.sql":              true,
	"1787444866_entity_type_admits_partner.up.sql":                       true,
	"1787818336_audit_admits_a_hard_delete.up.sql":                       true,
	"1787831200_a_company_event_is_a_signal.up.sql":                      true,
	"1787840100_the_company_record_learns_what_the_company_runs.up.sql":  true,
	"1788302567_a_retry_knows_whose_mail_it_carries.up.sql":              true,
	"1788350000_a_verdict_says_how_sure_it_was.up.sql":                   true,
	"1788386600_remediation_work_is_not_buyer_activity.up.sql":           true,
	"1788489200_a_message_says_whether_it_asks_us_for_something.up.sql":  true,
	"1788560000_a_notice_the_installation_owes_is_not_engagement.up.sql": true,
	"1788642089_the_wizard_step_check_names_every_step.up.sql":           true,
	"1788650003_a_reply_says_whether_it_was_a_yes.up.sql":                true,
}

func TestAConstraintIsValidatedInItsOwnMigration(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("migrations", "core", "*.up.sql"))
	if err != nil {
		t.Fatalf("listing core migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no core migrations found — this gate would pass over an empty tree")
	}

	seen := map[string]bool{}
	for _, path := range files {
		raw, err := os.ReadFile(path) // #nosec G304 -- a repo-relative migration path from the glob above
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		sql := withoutSQLComments(string(raw))
		if !addsNotValid.MatchString(sql) || !validatesInPlace.MatchString(sql) {
			continue
		}
		name := filepath.Base(path)
		seen[name] = true
		if sameTransactionValidators[name] {
			continue
		}
		t.Errorf("%s adds a constraint NOT VALID and validates it in the same file.\n\n"+
			"dbmigrate runs a file and its ledger row in ONE transaction, so the ACCESS EXCLUSIVE the "+
			"ALTER took is held until commit and the VALIDATE's lighter lock changes nothing — the "+
			"two-step costs an extra statement and buys the outage it was written to avoid. Put the "+
			"VALIDATE in a migration of its own; "+
			"1787968163_the_brief_currency_check_is_validated.up.sql is the worked example", name)
	}

	// The register only shrinks. An entry that stops matching is a migration
	// somebody edited — which the ledger's content digest refuses on every
	// database that ran it — or one this scan stopped reading, and the second
	// is how a census quietly starts covering nothing.
	for name := range sameTransactionValidators {
		if !seen[name] {
			t.Errorf("%s is registered as an applied same-transaction validator but no longer matches — "+
				"either it was edited, which the ledger digest refuses, or this scan no longer reads it",
				name)
		}
	}
}

// withoutSQLComments strips `--` comments so the scan reads statements. These
// files explain the NOT VALID pattern in prose at length, and every one of the
// registered fourteen says "NOT VALID" in a comment as well as performing it.
func withoutSQLComments(sql string) string {
	var out strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		if at := strings.Index(line, "--"); at >= 0 {
			line = line[:at]
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}
