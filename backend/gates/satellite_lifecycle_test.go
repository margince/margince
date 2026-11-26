// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every contact_ table with a contact_id column is reached by the retention
// anonymizer and the archive cascade, or says why not.
// piicoverage_test.go proves Art. 17 erasure and Art. 15 SAR reach every table
// its registry declares PII-bearing. The merge is held per column by
// mergereferences_test.go. These are the other two paths a contact's child
// rows ride, and where a new satellite rots unseen. A satellite nobody
// archived stays live under an archived contact. One the anonymizer missed
// keeps a copy of the subject.
//
// The obligations are derived, never listed:
//
//   - The satellite set comes from the migration DDL: every contact_ table
//     with a contact_id column. A new satellite is enrolled by the migration
//     that creates it, not by an edit here.
//   - The archive cascade binds a satellite IFF it has an archived_at column.
//     Four satellites (contact_consent, contact_social, contact_profile_field,
//     contact_signature_enrich_state) have none, so ArchiveContact has nothing
//     to soft-delete on them: their rows leave the database only when the
//     contact ROW itself is deleted, through the contact FK's
//     ON DELETE CASCADE. ArchiveContact is a SOFT delete, so it does not fire
//     that cascade, and no path in this tree hard-deletes a contact — those
//     four rows therefore outlive the archive. That is a real coverage gap in
//     the lifecycle, not a discharge of it; what this gate can honestly hold
//     is only the obligation the table's own shape admits, and demanding a
//     soft delete on a table with no archived_at column would be red for a
//     requirement that does not apply.
//   - The retention anonymizer binds a satellite that the PII registry
//     (piiTables) declares subject-bearing. Registration in that registry is
//     the ONE act that declares a table holds a data subject, and it already
//     carries the ratified reasons a contact_* table may sit outside it —
//     contact_consent, for instance, is deliberately kept under Art. 5
//     accountability rather than erased. Re-deciding that here would fork the
//     judgment across two gates.
//
// Presence is the whole check: each path's corpus is what its entry point can
// REACH through the package call graph, reusing the write-target extraction the
// ownership gate already spells (sqlWriteTargets). It proves the table is
// WRITTEN by that path, not that the write is correct — semantics belong to the
// module's own tests. What it catches is the silent omission.
//
// REACH, never a filename. A path keyed to the file its statements sit in today
// reports a dropped obligation the moment somebody splits a long function, which
// is a constraint nobody chose and a failure that fires when nothing is wrong.

import (
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// satellitePath is one lifecycle obligation: the entry point that discharges
// it, and the message a missing table gets. Every path here is a WRITE —
// archiving or deleting the satellite's rows.
type satellitePath struct {
	name string
	// pkg and from together name the path's corpus: everything the entry point
	// reaches inside its own package.
	pkg    string
	from   string
	remedy string
	// archivedOnly restricts the path to satellites carrying archived_at.
	archivedOnly bool
	// piiOnly restricts the path to satellites the PII registry declares
	// subject-bearing.
	piiOnly bool
}

// satelliteLifecyclePaths are the contact-satellite obligations this gate owns.
// Art. 17 erasure and Art. 15 SAR are deliberately absent: piicoverage_test.go
// already binds them to the same registry piiOnly reads, and duplicating them
// here would mean two gates to keep in step over one promise.
var satelliteLifecyclePaths = []satellitePath{
	{
		name:         "archive_cascade",
		pkg:          "internal/modules/contacts",
		from:         "Store.ArchiveContact",
		remedy:       "archive its rows somewhere ArchiveContact reaches — an unlisted satellite stays LIVE under an archived Contact",
		archivedOnly: true,
	},
	{
		name:    "retention_anonymize",
		pkg:     "internal/modules/privacy",
		from:    "RetentionService.anonymizeContact",
		remedy:  "delete its rows somewhere the contact/anonymize executor reaches — the sweep anonymizes the contact row and would leave this satellite's copy of the subject behind",
		piiOnly: true,
	},
}

// portCarry names who carries one satellite through a port: the owning
// module's implementation, and the contacts function that decides whether an
// UNWIRED merge would strand a row of it and so must refuse.
//
// The reach above is ONE package's call graph and stops at a port on purpose:
// following it would mean modelling the wiring, and a gate that models wiring
// agrees with itself rather than with the tree. So the register is checked
// from both ends instead, by TestEveryPortCarriesTheTablesItsRegisterEntryClaims.
type portCarry struct {
	pkg     string
	carrier string
	refusal string
}

const (
	consentPkg       = "internal/modules/consent"
	identityPkg      = "internal/modules/identity"
	initiativesPkg   = "internal/modules/initiatives"
	introductionsPkg = "internal/modules/introductions"
	signalsPkg       = "internal/modules/signals"
)

// carriedThroughAPort names the satellites another module moves inside the
// merge's own transaction.
var carriedThroughAPort = map[string]portCarry{
	"communication_suppression":  {consentPkg, "Store.CarryStopsTx", "holdsALiveStop"},
	"communication_override":     {consentPkg, "Store.CarryOverridesTx", "holdsALiveOverride"},
	"withdrawal_credential":      {consentPkg, "Store.CarrySatellitesTx", "holdsAConsentSatellite"},
	"preference_token":           {consentPkg, "Store.CarrySatellitesTx", "holdsAConsentSatellite"},
	"confirm_token":              {consentPkg, "Store.CarrySatellitesTx", "holdsAConsentSatellite"},
	"communication_basis":        {consentPkg, "Store.CarrySatellitesTx", "holdsAConsentSatellite"},
	"consent_qualifying_event":   {consentPkg, "Store.CarrySatellitesTx", "holdsAConsentSatellite"},
	"consent_doi_token":          {consentPkg, "Store.CarrySatellitesTx", "holdsAConsentSatellite"},
	"data_subject_request":       {consentPkg, "Store.BindRequestsToAddressesTx", "holdsAConsentSatellite"},
	"signal":                     {signalsPkg, "CarryResolutionsTx", "refuseUnwiredSignalCarry"},
	"intro_request":              {introductionsPkg, "Store.CarryIntrosTx", "refuseUnwiredIntroCarry"},
	"record_grant":               {identityPkg, "Service.CarryGrantsTx", "refuseUnwiredGrantCarry"},
	"sales_initiative_member":    {initiativesPkg, "CarryLeadMergeTx", "refuseUnwiredInitiativeCarry"},
	"sales_initiative_enrolment": {initiativesPkg, "CarryLeadMergeTx", "refuseUnwiredInitiativeCarry"},
}

var (
	// contactSatelliteName matches the CREATE TABLE lines this gate governs:
	// child tables named for the contact they hang off.
	contactSatelliteName = regexp.MustCompile(`^contact_[a-z_]+$`)
	// columnLine matches a bare column definition inside a CREATE TABLE block.
	// Constraint clauses (CONSTRAINT/PRIMARY/UNIQUE/CHECK/FOREIGN) never reach
	// the two names this gate reads, so no exclusion is needed.
	columnLine = regexp.MustCompile(`^\s+([a-z_]+)\s+[a-z]`)
	// alterColumn matches a later migration adding or dropping a column on an
	// existing table — contact_consent gains lead_id that way (0056), so a
	// derivation that read only CREATE TABLE would be reading a stale schema.
	alterColumn = regexp.MustCompile(`(?i)^\s*ALTER TABLE ([a-z_]+)\s+(ADD|DROP) COLUMN (?:IF (?:NOT )?EXISTS )?([a-z_]+)`)
	// dropTable matches a later migration removing a table outright, so a
	// dropped table stops being a satellite that owes a lifecycle path.
	dropTable = regexp.MustCompile(`(?i)^\s*DROP TABLE (?:IF EXISTS )?([a-z_]+)`)
)

// contactSatellites derives the governed satellites from the migration sources:
// table name → its column set. A contact_*-named CREATE TABLE with a contact_id
// column qualifies; ADD/DROP COLUMN and DROP TABLE in a later migration are
// folded in, in file order, so the column set is the one the migrated schema
// actually has.
func contactSatellites(t *testing.T) map[string]map[string]bool {
	t.Helper()
	columns := map[string]map[string]bool{}
	var paths []string
	for _, root := range []string{"migrations/core", "migrations/custom"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
				return err
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// core is sequentially numbered and custom timestamp-ordered, so a
	// lexical sort per namespace IS apply order (ADR-0017).
	sort.Strings(paths)
	for _, path := range paths {
		raw, err := gatekit.ReadFile(path) // #nosec G304 G122 -- path is a *.up.sql file from walking the trusted migrations tree
		if err != nil {
			t.Fatal(err)
		}
		current := ""
		for line := range strings.SplitSeq(withCurrentNames(string(raw)), "\n") {
			if m := createTableLine.FindStringSubmatch(line); m != nil {
				current = m[1]
				columns[current] = map[string]bool{}
				continue
			}
			if strings.HasPrefix(line, ");") {
				current = ""
				continue
			}
			if current != "" {
				if m := columnLine.FindStringSubmatch(line); m != nil {
					columns[current][m[1]] = true
				}
				continue
			}
			if m := dropTable.FindStringSubmatch(line); m != nil {
				delete(columns, m[1])
				continue
			}
			m := alterColumn.FindStringSubmatch(line)
			if m == nil || columns[m[1]] == nil {
				continue
			}
			if strings.EqualFold(m[2], "ADD") {
				columns[m[1]][m[3]] = true
			} else {
				delete(columns[m[1]], m[3])
			}
		}
	}
	satellites := map[string]map[string]bool{}
	for table, cols := range columns {
		if cols["contact_id"] {
			satellites[table] = cols
		}
	}
	// The derivation reads DDL text, so a change in how migrations spell a
	// CREATE TABLE would empty it silently and the gate would pass by finding
	// nothing to check. contact_email and contact_phone have been satellites
	// since 0004 and cannot legitimately disappear.
	for _, expected := range []string{"contact_email", "contact_phone"} {
		if satellites[expected] == nil {
			t.Fatalf("derived no %s satellite from the migrations — the derivation is broken, not the schema", expected)
		}
	}
	return satellites
}

func TestEveryContactSatelliteJoinsEveryLifecyclePathThatApplies(t *testing.T) {
	t.Parallel()

	satellites := contactSatellites(t)
	var missing []string
	for _, path := range satelliteLifecyclePaths {
		writes := path.writes(t)
		for table, cols := range satellites {
			if !contactSatelliteName.MatchString(table) {
				continue // outside the prefix this path's corpus is drawn from
			}
			if path.archivedOnly && !cols["archived_at"] {
				continue // removed by the contact FK cascade; nothing to archive
			}
			if _, subjectBearing := piiTables[table]; path.piiOnly && !subjectBearing {
				continue // not declared subject-bearing in piiTables
			}
			if writes[table] {
				continue
			}
			missing = append(missing, "contact satellite "+table+" is not handled by the "+path.name+
				" path ("+path.where()+") — "+path.remedy)
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Error(m)
	}
}

// writes returns the tables this path writes: the SQL literals of one file, or
// everything its entry point can reach.
func (p satellitePath) writes(t *testing.T) map[string]bool {
	t.Helper()
	return reachedWrites(t, p.pkg, p.from)
}

// where names the path in a finding, so a reader knows where to go.
func (p satellitePath) where() string {
	return p.from + " and what it calls, in " + p.pkg
}

// reachedWrites collects the tables written by `from` and by anything it calls,
// reading the package's own call graph.
//
// A CROSS-MODULE CARRY IS INVISIBLE TO IT, on purpose: the graph is one
// package's, so a satellite another module owns and moves through a port —
// communication_suppression, through StopCarrier — does not appear here and has
// to say so in the register. That is the honest shape. A reach that followed
// ports would have to model the wiring, and a gate that models wiring agrees
// with itself rather than with the tree.
func reachedWrites(t *testing.T, dir, from string) map[string]bool {
	t.Helper()
	writes := map[string]bool{}
	for _, statement := range reachedStatements(t, dir, from) {
		for _, table := range sqlWriteTargets(statement) {
			writes[table] = true
		}
	}
	return writes
}

// reachedStatements returns the SQL literals of `from` and of everything it
// calls inside its own package.
func reachedStatements(t *testing.T, dir, from string) []string {
	t.Helper()
	graph := packageCallGraph(t, dir)
	var statements []string
	for name := range reachedFunctions(t, graph, dir, from) {
		statements = append(statements, graph[name].statements...)
	}
	return statements
}

// reachedFunctions returns `from` and every function of the graph it calls,
// directly or not.
func reachedFunctions(t *testing.T, graph map[string]*graphFunc, dir, from string) map[string]bool {
	t.Helper()
	if _, known := graph[from]; !known {
		t.Fatalf("no function %s in %s — the entry point was renamed and this census now "+
			"reads an empty corpus, which is PASS for every table", from, dir)
	}
	reached := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		fn, known := graph[name]
		if reached[name] || !known {
			return
		}
		reached[name] = true
		for called := range fn.calls {
			walk(called)
		}
	}
	walk(from)
	return reached
}

// TestEveryPortCarriesTheTablesItsRegisterEntryClaims holds carriedThroughAPort
// to the tree from both ends: the carrier writes the table, and the refusal an
// unwired merge runs reads it. A carrier that stopped writing a table leaves it
// on the retired record; a refusal that stopped reading one lets an unwired
// merge strand it silently.
func TestEveryPortCarriesTheTablesItsRegisterEntryClaims(t *testing.T) {
	t.Parallel()
	referencing := map[string]bool{}
	catalog := headCatalogLines(t)
	for _, merged := range mergedRecords {
		for _, column := range referencesTo(catalog, merged.record) {
			table, _, _ := strings.Cut(column, ".")
			referencing[table] = true
		}
	}
	refusals := packageCallGraph(t, "internal/modules/contacts")
	for table, port := range carriedThroughAPort {
		if !referencing[table] {
			t.Errorf("carriedThroughAPort names %s, which has no column referencing a contact or a lead — "+
				"an entry the census never asks about looks like coverage and answers nothing", table)
			continue
		}
		if !reachedWrites(t, port.pkg, port.carrier)[table] {
			t.Errorf("%s.%s is registered as carrying %s onto the survivor and writes no such table — "+
				"a merge would leave its rows on the retired record", port.pkg, port.carrier, table)
		}
		refusal, known := refusals[port.refusal]
		if !known {
			t.Fatalf("no function %s in internal/modules/contacts — the refusal was renamed and this "+
				"check now reads nothing", port.refusal)
		}
		if !readsTable(refusal.statements, table) {
			t.Errorf("contacts.%s decides whether an unwired merge would strand a row and never reads %s — "+
				"a merge on an installation without the %s seam would drop those rows without refusing",
				port.refusal, table, port.pkg)
		}
	}
}

// readsTable reports whether any statement names table as a FROM target.
func readsTable(statements []string, table string) bool {
	from := regexp.MustCompile(`(?i)\bFROM\s+` + regexp.QuoteMeta(table) + `\b`)
	return slices.ContainsFunc(statements, from.MatchString)
}
