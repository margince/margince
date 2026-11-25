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
// column at a time (referencesTo). Some columns the catalog keeps equal to
// another: a _bound CHECK, a generated column or an ON UPDATE CASCADE key.
// Such a column is judged as that other column. A column counts as carried
// when:
//
//   - an UPDATE, or an INSERT ... SELECT, that the merge reaches in contacts
//     writes it;
//   - a port in carriedThroughAPort writes it, and the merge reaches the
//     contacts function that refuses when that port is not wired;
//   - a declared link moves it (sweepreach);
//   - carriedOutOfSight names the contacts function that moves it.
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
	{"contact", "Store.MergeContact", 78},
	{"lead", "Store.MergeLead", 45},
}

// carriedOutOfSight names, for each column a merge moves in a statement this
// scan cannot read, the function holding that statement. A contacts function
// must be one the merge reaches and must name the table. A "pkg:Func" entry is
// another package's function: a function the merge reaches names it, and its
// statements write the table.
var carriedOutOfSight = map[string]string{
	// Only the provenance of the fields the survivor took from the retired
	// record moves. The rest describes a value the merge did not keep.
	"field_provenance.object_id":                storekitOwned + ":MoveFieldProvenance",
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

// mergeHistory is the reason shared by the rows that record what happened to
// the retired record.
const mergeHistory = "history of the retired record, which is never rewritten"

// stayOnTheRetiredRecord holds the columns a merge leaves where they are, and
// why. Keys are table.column.
var stayOnTheRetiredRecord = gatekit.Waive(map[string]string{
	"contact_address_country.contact_id":       "a check of the merged-away contact's own postal code and city, which says nothing about the survivor's address. The survivor keeps its own check, and the trigger deletes it when the merge moves the survivor's address; the next sweep checks the address the survivor ends with",
	"contact_brief.contact_id":                 "a cached sentence about one record. relinkReaderJudgements deletes the briefs of both halves, so the next read writes one about the record that stands",
	"contact.converted_from_lead_id":           "names the lead a contact was promoted from. Only an open lead merges, and a promoted lead is archived, so no contact names a lead that a merge retires",
	"dedupe_candidate.left_contact_id":         "the duplicate pair the reviewer judged. The merge settles that pair and archives every other open pair naming the retired record, so each row stays the record of a decision about the two it named",
	"dedupe_candidate.right_contact_id":        "the duplicate pair the reviewer judged. The merge settles that pair and archives every other open pair naming the retired record, so each row stays the record of a decision about the two it named",
	"dedupe_candidate.left_lead_id":            "the duplicate pair the reviewer judged. retireStaleCandidates archives every other open pair naming the retired lead, so each row stays the record of a decision about the two it named",
	"dedupe_candidate.right_lead_id":           "the duplicate pair the reviewer judged. retireStaleCandidates archives every other open pair naming the retired lead, so each row stays the record of a decision about the two it named",
	"graph_interaction_edge.contact_id":        "search owns it and rebuilds it: graphedgegen.go consumes contact.merged and refolds the survivor's edges after dropping the source's. The table is derived from activities the merge has already moved, so a moved row would carry a fold of the old graph",
	"lawful_basis_confirmation.contact_id":     "a confirmation that lasts 15 minutes and is spent by the next send to that record. Left on the retired record it expires unspent, and a send to the survivor asks again. Moving it would let one confirmation answer a send to a record the user did not confirm",
	"lawful_basis_confirmation.lead_id":        "a confirmation that lasts 15 minutes and is spent by the next send to that record. Left on the retired record it expires unspent, and a send to the survivor asks again. Moving it would let one confirmation answer a send to a record the user did not confirm",
	"lead_privacy_notice.lead_id":              "the notice sent for the capture that made the retired lead. refuseWebsiteLeadNotYetTold refuses the merge until both leads are told, and the table holds one notice per lead, so the survivor's own notice is the one that speaks for it",
	"lead_score_history.lead_id":               "the scores the retired lead had, each computed from what it held then. The merge scores the survivor again from what it now holds, and that score starts the survivor's own history",
	"tag_suggestion.contact_id":                "the tag scout's reading of one record's evidence. The next scout pass reads the archived contact's suggestion as superseded, and the activities the merge moved raise the survivor's own. A moved row could collide with the survivor's open suggestion of the same tag",
	"user_record_view.entity_id":               "when one user last opened the retired record. The survivor keeps its own stamp, and a moved stamp would say the user saw the survivor's changes when they had opened another record",
	"ai_call.subject_id":                       "the log of one model call made about the retired record: " + mergeHistory,
	"ai_task_run.subject_id":                   "a run of an AI task about the retired record, computed from what it held then: " + mergeHistory,
	"approval.target_entity_id":                "a person's approval of one action on the record it named. Moved, an approval given for the retired record would act on the survivor; confirm links are expired rather than moved for the same reason",
	"approval.co_target_entity_id":             "a person's approval of one action on the records it named. Moved, an approval given for the retired record would act on the survivor; confirm links are expired rather than moved for the same reason",
	"audit_log.entity_id":                      "the audit trail of the retired record: " + mergeHistory,
	"chat_post_item.event_id":                  "names a posted event, not a record. The kind column holds event types, and no CHECK lists them, so the census asks",
	"communication_decision.subject_id":        "what the send engine decided for the retired record at the time: " + mergeHistory,
	"embedding.entity_id":                      "the search index's vector of the retired record's text. The indexer embeds the survivor from its own text, and a moved row would index the retired record's text under the survivor",
	"list_live_member.entity_id":               "the cached result of a live list's filter. collections evaluates the list again from the records as they now are, so the survivor is a member when it matches",
	"list_member_event.entity_id":              "the log of the retired record's list membership changes: " + mergeHistory + ". The membership itself moves (list_member)",
	"report_edition_contribution.source_id":    "what a report edition was built from: " + mergeHistory,
	"report_edition_question_source.source_id": "what a report edition's questions were built from: " + mergeHistory,
	"webhook_delivery.entity_id":               "the log of webhook deliveries already sent about the retired record: " + mergeHistory,
	"webhook_delivery.event_id":                "names a delivered event, not a record. The kind column holds event types, and no CHECK lists them, so the census asks",
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
		for _, column := range obligationsOf(references, mirroredColumns(catalog, merged.record)) {
			if carried[column] {
				continue
			}
			if by, ok := carriedOutOfSight[column]; ok {
				outOfSight[column] = true
				if err := movesOutOfSight(t, contacts, reached, by, column); err != nil {
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

// obligationsOf answers each reference by the column the catalog keeps it
// equal to, when it has one. So a key and its source are judged once.
func obligationsOf(references []string, mirrors map[string]string) []string {
	var columns []string
	for _, column := range references {
		if source, ok := mirrors[column]; ok {
			column = source
		}
		columns = append(columns, column)
	}
	slices.Sort(columns)
	return slices.Compact(columns)
}

// movesOutOfSight checks a carriedOutOfSight entry: the merge reaches the
// function, and one of its statements writes the table.
func movesOutOfSight(t *testing.T, contacts map[string]*graphFunc, reached map[string]bool, by, column string) error {
	table, _, _ := strings.Cut(column, ".")
	var statements []string
	if pkg, fn, elsewhere := strings.Cut(by, ":"); elsewhere {
		if !slices.ContainsFunc(slices.Collect(maps.Keys(reached)), func(name string) bool { return contacts[name].reads[fn] }) {
			return fmt.Errorf("no function the merge reaches names %s", fn)
		}
		statements = reachedStatements(t, pkg, fn)
	} else if reached[by] {
		statements = contacts[by].statements
	} else {
		return fmt.Errorf("the merge never reaches contacts.%s", by)
	}
	for _, statement := range statements {
		if slices.Contains(sqlWriteTargets(statement), table) {
			return nil
		}
	}
	return fmt.Errorf("%s writes no %s statement", by, table)
}

// headCatalogLines reads the head catalog with each multi-line definition
// folded onto the line that opens it (catalogEntryStart).
func headCatalogLines(t *testing.T) []string {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if catalogEntryStart.MatchString(line) || len(lines) == 0 {
			lines = append(lines, line)
			continue
		}
		lines[len(lines)-1] += " " + strings.TrimSpace(line)
	}
	return lines
}

// referencesTo returns every table.column that can name the record:
//   - a foreign key to it;
//   - a column named for it with no key;
//   - the id of a kind and id pair whose kind admits it;
//   - a link a module declares for the merge.
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
	columns = append(columns, kindPairsAdmitting(catalog, record)...)
	for _, o := range corereach.Tables() {
		for _, l := range o.Table.Links {
			if l.KindColumn != "" || string(l.Record) == record {
				columns = append(columns, o.Table.Name+"."+l.Column)
			}
		}
	}
	slices.Sort(columns)
	return slices.Compact(columns)
}

// kindPairsAdmitting returns the id column of every kind and id pair (a text
// <name>_type or <name>_kind beside a uuid <name>_id) that can name the
// record. A pair counts unless a CHECK on its kind column lists the kinds and
// leaves the record out. So a pair with no such CHECK counts.
func kindPairsAdmitting(catalog []string, record string) []string {
	columns := map[string]map[string]string{}
	checks := map[string][]string{}
	for _, line := range catalog {
		if m := catalogColumn.FindStringSubmatch(line); m != nil {
			if columns[m[1]] == nil {
				columns[m[1]] = map[string]string{}
			}
			columns[m[1]][m[2]] = strings.Fields(m[3])[0]
		}
		if m := catalogConstraint.FindStringSubmatch(line); m != nil && strings.HasPrefix(m[3], "CHECK") {
			checks[m[1]] = append(checks[m[1]], m[3])
		}
	}
	var pairs []string
	for table, cols := range columns {
		for kind, typ := range cols {
			stem, isKind := strings.CutSuffix(kind, "_type")
			if !isKind {
				stem, isKind = strings.CutSuffix(kind, "_kind")
			}
			if !isKind || typ != "text" || cols[stem+"_id"] != "uuid" {
				continue
			}
			if admitsKind(checks[table], kind, record) {
				pairs = append(pairs, table+"."+stem+"_id")
			}
		}
	}
	return pairs
}

// admitsKind reports whether the kind column may hold the record: no CHECK
// lists its values, or one that does names the record.
func admitsKind(checks []string, kindColumn, record string) bool {
	lists := regexp.MustCompile(`\b` + kindColumn + `\)? = (ANY|')`)
	listed := false
	for _, check := range checks {
		if lists.MatchString(check) {
			listed = true
			if strings.Contains(check, "'"+record+"'") {
				return true
			}
		}
	}
	return !listed
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
