// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Contact-satellite lifecycle reach as a fitness function. piicoverage_test.go
// proves Art. 17 erasure and Art. 15 SAR reach every table its registry
// declares PII-bearing; it says nothing about the three OTHER lifecycle paths a
// contact's child rows ride — the retention anonymizer, the merge relink, and the
// archive cascade. Those are where a new satellite rots invisibly: a satellite
// nobody archived stays live under an archived Contact, and one nobody relinked
// is orphaned on the merged-away half. Neither errors, and neither is visible
// until someone reads the row that should be gone.
//
// The obligations are DERIVED, never listed:
//
//   - The satellite set comes from the migration DDL — every table with a
//     contact_id column. A new satellite is enrolled by the migration that
//     creates it, not by an edit here. Two of the three paths below then narrow
//     it to the contact_ prefix, because their obligations are about the
//     contact's OWN child rows; the merge does not, because a merge orphans
//     whatever names the retired id whatever it is called. Sixteen tables were
//     invisible to this census while the prefix was the whole of it.
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
//     (piiTables) declares subject-bearing. The merge relink does NOT, and the
//     difference is the point: an anonymizer owes only what carries the
//     subject, while a merge owes every row pointing at the record it retires —
//     a dismissal that silently comes back is not PII and is still a defect.
//     Registration in that registry is the ONE act that declares a table holds
//     a data subject, and it already carries the ratified reasons a contact_*
//     table may sit outside it — contact_consent, for instance, is deliberately
//     kept under Art. 5 accountability rather than erased. Re-deciding that here
//     would fork the judgment across two gates.
//   - The merge's own corpus is what relinkContactReferences can REACH, read
//     from the package call graph rather than from one file's SQL. The merge
//     spans three files today and a gate naming them would go quiet the day a
//     fourth appeared.
//
// Presence is the whole check: each path is a source-text scan of the file that
// discharges it, reusing the write-target extraction the ownership gate already
// spells (sqlWriteTargets). It proves the table is WRITTEN by that path, not
// that the write is correct — semantics belong to the module's own tests. What
// it catches is the silent omission.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// satellitePath is one lifecycle obligation: the file that discharges it, and
// the message a missing table gets. Every path here is a WRITE — archiving,
// deleting or relinking the satellite's rows.
type satellitePath struct {
	name   string
	file   string
	remedy string
	// archivedOnly restricts the path to satellites carrying archived_at.
	archivedOnly bool
	// piiOnly restricts the path to satellites the PII registry declares
	// subject-bearing.
	piiOnly bool
	// everyTableNamingAContact widens the corpus past the contact_ prefix to
	// every table carrying a contact_id column.
	everyTableNamingAContact bool
	// from names the function whose call-graph reach is the path's corpus,
	// instead of `file`'s SQL literals. Exactly one of the two is set.
	from string
}

// satelliteLifecyclePaths are the contact-satellite obligations this gate owns.
// Art. 17 erasure and Art. 15 SAR are deliberately absent: piicoverage_test.go
// already binds them to the same registry piiOnly reads, and duplicating them
// here would mean two gates to keep in step over one promise.
var satelliteLifecyclePaths = []satellitePath{
	{
		name:         "archive_cascade",
		file:         "internal/modules/contacts/contactarchive.go",
		remedy:       "add it to ArchiveContact's statement list — an unlisted satellite stays LIVE under an archived Contact",
		archivedOnly: true,
	},
	{
		name:    "retention_anonymize",
		file:    "internal/modules/privacy/retentionactions.go",
		remedy:  "delete its rows in the contact/anonymize executor — the sweep anonymizes the contact row and would leave this satellite's copy of the subject behind",
		piiOnly: true,
	},
	{
		name:   "merge_relink",
		remedy: "relink its rows onto the survivor from relinkContactReferences — rows left on the merged-away contact are orphaned, invisible to every read of the survivor",
		// EVERY TABLE POINTING AT THE CONTACT, not only the subject-bearing
		// ones, and not only those NAMED for the contact. A merge orphans
		// whatever still names the retired id — a dismissal that silently comes
		// back, a credential that keeps working against a record no read
		// returns — and neither the PII registry nor the contact_ prefix has
		// anything to say about that. The two paths above are narrower for
		// reasons of their own: an anonymizer owes only what carries the
		// subject, and an archive only what has an archived_at to set.
		everyTableNamingAContact: true,
		// REACHED, not read from one file. The merge's relinks live in
		// mergerelink.go, its consent carry in consentcarry.go and its stop
		// carry behind a port in stopcarry.go, and a gate naming those three
		// files would be a second copy of where merge code happens to sit — it
		// would go quiet the day a fourth appeared. The corpus is what
		// relinkContactReferences can reach instead.
		from: "relinkContactReferences",
	},
}

// carriedElsewhere ratifies a table the merge does not write from
// relinkContactReferences. Every entry says WHO moves it instead, or why
// nothing should — and an entry the census never asks about is reported as
// unmatched, so a ratification cannot outlive the reason for it.
var carriedElsewhere = gatekit.Waive(map[string]string{
	"communication_suppression": "carried by consent, through the StopCarrier port (contacts/stopcarry.go), inside the merge's own transaction. The reach above is ONE package's call graph and stops at the port on purpose: following it would mean modelling the wiring, and a gate that models wiring agrees with itself rather than with the tree. What holds the carry instead is consent's own CarryStopsTx and the merge's refusal to proceed at all when the seam is unwired and the subject holds a live stop",
	"graph_interaction_edge":    "search owns it and REBUILDS it rather than moving it: graphedgegen.go consumes contact.merged and refolds the survivor's edges after dropping the source's, which is the right shape for a table derived entirely from activities the merge has already relinked. Moving the rows instead would carry a fold computed against the pre-merge graph",
})

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
)

// contactSatellites derives the governed satellites from the migration sources:
// table name → its column set. A contact_*-named CREATE TABLE with a contact_id
// column qualifies; ADD/DROP COLUMN in a later migration is folded in, in file
// order, so the column set is the one the migrated schema actually has.
func contactSatellites(t *testing.T) map[string]map[string]bool {
	t.Helper()
	columns := map[string]map[string]bool{}
	var paths []string
	for _, root := range []string{"migrations/core", "migrations/custom"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		raw, err := os.ReadFile(path) // #nosec G304 G122 -- path is a *.up.sql file from walking the trusted migrations tree
		if err != nil {
			t.Fatal(err)
		}
		current := ""
		for _, line := range strings.Split(withCurrentNames(string(raw)), "\n") {
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

// pathWrites returns the tables one lifecycle file writes.
func pathWrites(t *testing.T, file string) map[string]bool {
	t.Helper()
	writes := map[string]bool{}
	for _, lit := range sqlLiterals(t, file) {
		for _, table := range sqlWriteTargets(lit) {
			writes[table] = true
		}
	}
	return writes
}

// notYetCarried is NOT a ratification. It is a list of tables the merge does
// not carry and SHOULD, kept apart from carriedElsewhere on purpose: that
// register says who moves a row instead, and an entry saying "nobody, yet"
// dressed as one would make this census report a clean merge over a defect it
// can see. These are the defect, named.
//
// Each needs a port in the module that owns the table AND a decision only that
// module can make — whether a live credential follows the survivor or is
// revoked, whether a §7(3) flag one half held may widen who the survivor may be
// mailed about. Tracked as #5771.
//
// CLOSED to new entries. It records what this census found when it was widened
// to see them at all — before that, every one of these was invisible to it —
// and it only shrinks. A table leaves when its owner carries it.
var notYetCarried = gatekit.Waive(map[string]string{
	"communication_basis":            "#5771 — the module that owns it has no carry port yet",
	"confirm_token":                  "#5771 — the module that owns it has no carry port yet",
	"consent_doi_token":              "#5771 — the module that owns it has no carry port yet",
	"consent_existing_customer_flag": "#5771 — the module that owns it has no carry port yet",
	"consent_qualifying_event":       "#5771 — the module that owns it has no carry port yet",
	"preference_token":               "#5771 — the module that owns it has no carry port yet",
	"withdrawal_credential":          "#5771 — the module that owns it has no carry port yet",
	"intro_request":                  "#5771 — the module that owns it has no carry port yet",
})

func TestEveryContactSatelliteJoinsEveryLifecyclePathThatApplies(t *testing.T) {
	t.Parallel()
	defer carriedElsewhere.AssertAllMatched(t)
	defer notYetCarried.AssertAllMatched(t)

	satellites := contactSatellites(t)
	var missing []string
	for _, path := range satelliteLifecyclePaths {
		writes := path.writes(t)
		for table, cols := range satellites {
			if !path.everyTableNamingAContact && !contactSatelliteName.MatchString(table) {
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
			// ASKED ABOUT AN OFFENDER, never about a candidate: a table the
			// path already writes never reaches the register, so a stale
			// ratification shows up as unmatched rather than sitting there
			// agreeing with a path that no longer needs it.
			if path.name == mergeRelinkPath && carriedElsewhere.Waived(t, table) {
				continue
			}
			// KNOWN AND UNFIXED, which is a different answer from discharged —
			// see notYetCarried.
			if path.name == mergeRelinkPath && notYetCarried.Waived(t, table) {
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

// mergeRelinkPath is the one path whose corpus is every table naming a contact,
// and so the one whose ratifications the register above answers for.
const mergeRelinkPath = "merge_relink"

// writes returns the tables this path writes: the SQL literals of one file, or
// everything its entry point can reach.
func (p satellitePath) writes(t *testing.T) map[string]bool {
	t.Helper()
	if p.from == "" {
		return pathWrites(t, p.file)
	}
	return reachedWrites(t, "internal/modules/contacts", p.from)
}

// where names the path in a finding, so a reader knows where to go.
func (p satellitePath) where() string {
	if p.from == "" {
		return p.file
	}
	return p.from + " and what it calls"
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
	graph := packageCallGraph(t, dir)
	if _, known := graph[from]; !known {
		t.Fatalf("no function %s in %s — the merge entry point was renamed and this census now "+
			"reads an empty corpus, which is PASS for every table", from, dir)
	}
	writes := map[string]bool{}
	seen := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		fn, known := graph[name]
		if !known {
			return
		}
		for _, statement := range fn.statements {
			for _, table := range sqlWriteTargets(statement) {
				writes[table] = true
			}
		}
		for called := range fn.calls {
			walk(called)
		}
	}
	walk(from)
	return writes
}
