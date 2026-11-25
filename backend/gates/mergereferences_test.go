// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A merge of a contact or a lead carries every column that references it, or
// the register says why the row stays.
//
// A row a merge leaves on the retired record is reachable from no read of the
// survivor, and nothing fails. So the obligation comes from the catalog, one
// column at a time. It takes every foreign key to the record, whatever its
// name, and every column named for the record that has no foreign key. A
// column counts as carried when:
//
//   - an UPDATE, or an INSERT ... SELECT, that the merge reaches in contacts
//     writes it;
//   - a port in carriedThroughAPort writes it, and the merge reaches the
//     contacts function that refuses when that port is not wired;
//   - a declared link moves it (sweepreach);
//   - carriedOutOfSight names the contacts function that moves it;
//   - the catalog keeps it equal to another column of its row (a _bound CHECK,
//     a generated column or an ON UPDATE CASCADE key), and that column is
//     carried.
//
// This proves a statement moves the column, not that it moves every row; the
// owning module's tests hold that.

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/corereach"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const contactsPkg = "internal/modules/contacts"

// mergedRecords are the record kinds a merge retires, the contacts function
// that runs each merge, and the fewest columns the catalog has referencing each.
var mergedRecords = []struct {
	record, entry string
	floor         int
}{
	{"contact", "Store.MergeContact", 51},
	{"lead", "Store.MergeLead", 20},
}

// carriedOutOfSight names, for each column a merge moves in a statement this
// scan cannot read, the contacts function holding that statement. The
// function must be one the merge reaches and must name the table.
var carriedOutOfSight = map[string]string{
	"activity_link.contact_id":                  "relinkLinkRows",
	"consent_event.contact_id":                  "carryConsent",
	"consent_event.lead_id":                     "carryConsent",
	"contact_consent.contact_id":                "repointCarriedConsent",
	"contact_consent.lead_id":                   "repointCarriedConsent",
	"contact_profile_field.contact_id":          "relinkContactReferences",
	"contact_signature_enrich_state.contact_id": "relinkWorkInFlight",
	"relationship.contact_id":                   "mergeEdgeWrite",
	"relationship.counterparty_contact_id":      "mergeEdgeWrite",
}

// stayOnTheRetiredRecord holds the columns a merge leaves where they are, and
// why. Keys are table.column.
var stayOnTheRetiredRecord = gatekit.Waive(map[string]string{
	"contact_address_country.contact_id":   "a check of the merged-away contact's own postal code and city, which says nothing about the survivor's address. The survivor keeps its own check, and the trigger deletes it when the merge moves the survivor's address; the next sweep checks the address the survivor ends with",
	"contact_brief.contact_id":             "a cached sentence about one record. relinkReaderJudgements deletes the briefs of both halves, so the next read writes one about the record that stands",
	"contact.converted_from_lead_id":       "names the lead a contact was promoted from. Only an open lead merges, and a promoted lead is archived, so no contact names a lead that a merge retires",
	"dedupe_candidate.left_contact_id":     "the duplicate pair the reviewer judged. The merge settles that pair and archives every other open pair naming the retired record, so each row stays the record of a decision about the two it named",
	"dedupe_candidate.right_contact_id":    "the duplicate pair the reviewer judged. The merge settles that pair and archives every other open pair naming the retired record, so each row stays the record of a decision about the two it named",
	"dedupe_candidate.left_lead_id":        "the duplicate pair the reviewer judged. retireStaleCandidates archives every other open pair naming the retired lead, so each row stays the record of a decision about the two it named",
	"dedupe_candidate.right_lead_id":       "the duplicate pair the reviewer judged. retireStaleCandidates archives every other open pair naming the retired lead, so each row stays the record of a decision about the two it named",
	"graph_interaction_edge.contact_id":    "search owns it and rebuilds it: graphedgegen.go consumes contact.merged and refolds the survivor's edges after dropping the source's. The table is derived from activities the merge has already moved, so a moved row would carry a fold of the old graph",
	"lawful_basis_confirmation.contact_id": "a confirmation that lasts 15 minutes and is spent by the next send to that record. Left on the retired record it expires unspent, and a send to the survivor asks again. Moving it would let one confirmation answer a send to a record the user did not confirm",
	"lawful_basis_confirmation.lead_id":    "a confirmation that lasts 15 minutes and is spent by the next send to that record. Left on the retired record it expires unspent, and a send to the survivor asks again. Moving it would let one confirmation answer a send to a record the user did not confirm",
	"lead_privacy_notice.lead_id":          "the notice sent for the capture that made the retired lead. refuseWebsiteLeadNotYetTold refuses the merge until both leads are told, and the table holds one notice per lead, so the survivor's own notice is the one that speaks for it",
	"lead_score_history.lead_id":           "the scores the retired lead had, each computed from what it held then. The merge scores the survivor again from what it now holds, and that score starts the survivor's own history",
	"tag_suggestion.contact_id":            "the tag scout's reading of one record's evidence. The next scout pass reads the archived contact's suggestion as superseded, and the activities the merge moved raise the survivor's own. A moved row could collide with the survivor's open suggestion of the same tag",
	"user_record_view.entity_contact_id":   "when one user last opened the retired record. The survivor keeps its own stamp, and a moved stamp would say the user saw the survivor's changes when they had opened another record",
})

// The catalog keeps some columns equal to another column of the row. The
// caller joins each multi-line definition onto one line first.
var (
	// boundMirror is a CHECK holding the column equal to an id while a kind
	// column names the record.
	boundMirror = regexp.MustCompile(`^public\.(\w+)\.\w+_bound CHECK \(\(\w+ OR \(NOT \((\w+) IS DISTINCT FROM CASE WHEN \((\w+) = '(\w+)'::text\) THEN (\w+)`)
	// generatedMirror is a stored generated column computed the same way.
	generatedMirror = regexp.MustCompile(`^public\.(\w+)\.(\w+) uuid gen=s def= CASE WHEN \((\w+) = '(\w+)'::text\) THEN (\w+)`)
	// cascadedKey is a composite key that follows its parent row's columns.
	cascadedKey = regexp.MustCompile(`^public\.(\w+)\.\w+ FOREIGN KEY \(([\w, ]+)\) REFERENCES (\w+)\(([\w, ]+)\) ON UPDATE CASCADE`)
)

func TestEveryReferenceToAMergedRecordFollowsTheMerge(t *testing.T) {
	t.Parallel()
	defer stayOnTheRetiredRecord.AssertAllMatched(t)
	catalog := headCatalogLines(t)
	contacts := packageCallGraph(t, contactsPkg)
	outOfSight := map[string]bool{}
	for _, merged := range mergedRecords {
		references := referencesTo(catalog, merged.record)
		if len(references) < merged.floor {
			t.Fatalf("read %d columns referencing %s from the catalog, below the %d it has — the scan reads short, and a short census passes",
				len(references), merged.record, merged.floor)
		}
		carried := carriedBy(t, contacts, merged.entry, merged.record)
		reached := reachedFunctions(t, contacts, contactsPkg, merged.entry)
		mirrors := mirroredColumns(catalog, merged.record)
		for _, column := range references {
			if carried[column] || carried[mirrors[column]] {
				continue
			}
			if by, ok := carriedOutOfSight[column]; ok {
				outOfSight[column] = true
				if err := movesOutOfSight(contacts, reached, by, column); err != nil {
					t.Errorf("carriedOutOfSight[%q]: %v", column, err)
				}
				continue
			}
			if stayOnTheRetiredRecord.Waived(t, column) {
				continue
			}
			t.Errorf("%s references %s, and a %s merge leaves it on the retired record, where no read of the survivor finds it — "+
				"carry it from contacts.%s (a port in carriedThroughAPort for another module's table), or say in stayOnTheRetiredRecord why it stays",
				column, merged.record, merged.record, merged.entry)
		}
	}
	for _, column := range slices.Sorted(maps.Keys(carriedOutOfSight)) {
		if !outOfSight[column] {
			t.Errorf("carriedOutOfSight names %s, which the census found carried in sight or never asked about — remove the entry", column)
		}
	}
}

// movesOutOfSight checks a carriedOutOfSight entry: the merge reaches the
// function, and one of its statements writes the table.
func movesOutOfSight(contacts map[string]*graphFunc, reached map[string]bool, by, column string) error {
	if !reached[by] {
		return fmt.Errorf("the merge never reaches contacts.%s", by)
	}
	table, _, _ := strings.Cut(column, ".")
	for _, statement := range contacts[by].statements {
		if slices.Contains(sqlWriteTargets(statement), table) {
			return nil
		}
	}
	return fmt.Errorf("contacts.%s writes no %s statement", by, table)
}

// headCatalogLines reads the head catalog with each multi-line definition
// folded onto the line that opens it.
func headCatalogLines(t *testing.T) []string {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(line, "public.") || len(lines) == 0 {
			lines = append(lines, line)
			continue
		}
		lines[len(lines)-1] += " " + strings.TrimSpace(line)
	}
	return lines
}

// referencesTo returns every table.column with a foreign key to the record,
// and every column named for it that has none.
func referencesTo(catalog []string, record string) []string {
	named := regexp.MustCompile(`^public\.(\w+)\.(` + record + `_id) uuid `)
	var columns []string
	for _, line := range catalog {
		if m := foreignKeyToRowScoped.FindStringSubmatch(line); m != nil && m[3] == record {
			columns = append(columns, m[1]+"."+m[2])
		}
		if m := named.FindStringSubmatch(line); m != nil {
			columns = append(columns, m[1]+"."+m[2])
		}
	}
	slices.Sort(columns)
	return slices.Compact(columns)
}

// mirroredColumns maps each column the catalog keeps equal to another column
// of its row, for the rows naming this record, onto that column.
func mirroredColumns(catalog []string, record string) map[string]string {
	mirrors := map[string]string{}
	for _, line := range catalog {
		for _, pattern := range []*regexp.Regexp{boundMirror, generatedMirror} {
			if m := pattern.FindStringSubmatch(line); m != nil && m[4] == record {
				mirrors[m[1]+"."+m[2]] = m[1] + "." + m[5]
			}
		}
		if m := cascadedKey.FindStringSubmatch(line); m != nil {
			columns, parents := strings.Split(m[2], ", "), strings.Split(m[4], ", ")
			for i := range min(len(columns), len(parents)) {
				mirrors[m[1]+"."+columns[i]] = m[3] + "." + parents[i]
			}
		}
	}
	return mirrors
}

// carriedBy returns the table.column set a merge of the record moves. That is
// what its own statements and the ports it calls write, and the declared links.
func carriedBy(t *testing.T, contacts map[string]*graphFunc, entry, record string) map[string]bool {
	t.Helper()
	reached := reachedFunctions(t, contacts, contactsPkg, entry)
	carried := map[string]bool{}
	for name := range reached {
		movedColumns(carried, contacts[name].statements, nil)
	}
	for _, table := range slices.Sorted(maps.Keys(carriedThroughAPort)) {
		port := carriedThroughAPort[table]
		if reached[port.refusal] {
			movedColumns(carried, reachedStatements(t, port.pkg, port.carrier), map[string]bool{table: true})
		}
	}
	for _, o := range corereach.Tables() {
		for _, l := range o.Table.Links {
			if l.KindColumn != "" || string(l.Record) == record {
				carried[o.Table.Name+"."+l.Column] = true
			}
		}
	}
	return carried
}

// movedColumns adds every column the statements move: one an UPDATE sets, or
// one an INSERT ... SELECT fills, which is a merge copying rows across. An
// INSERT ... VALUES writes one new row and moves nothing. So a history row the
// merge appends for the survivor does not count. tables limits the scan when
// it is not nil.
func movedColumns(into map[string]bool, statements []string, tables map[string]bool) {
	for _, statement := range statements {
		for _, write := range sqlWrites(statement) {
			if tables != nil && !tables[write.table] {
				continue
			}
			copied := write.verb == "insert" && insertSelect.MatchString(statement)
			if write.verb != "update" && !copied {
				continue
			}
			for _, column := range write.cols {
				into[write.table+"."+column] = true
			}
		}
	}
}

// insertSelect matches a statement that fills an INSERT from a query.
var insertSelect = regexp.MustCompile(`(?is)\bINSERT\s+INTO\b.*\bSELECT\b`)
