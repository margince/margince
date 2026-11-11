// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every closed text vocabulary that a Go constant set mirrors is held to that
// set by a gate. A drift between the two then fails a test.
//
// The schema closes its text columns with `CHECK (col = ANY (ARRAY[…]))`, and
// the Go side spells the same words as typed string constants. enumsync binds
// a pair when someone writes a registry line for it. This census finds the
// pairs nobody wrote a line for. It reads both sides from the tree: the CHECKs
// from the committed head catalog, the constants from every Go source file.
//
// Three rules, one test each:
//
//  1. A Go type named for its column binds to it by name. The contract
//     generator names a schema after its table and column, so
//     contracts.VoiceBuildStatus is voice_build.status. Such a pair needs no
//     registry line: its two sets must be equal, or the column carries a
//     waiver saying why they differ.
//  2. A vocabulary whose value set equals some Go typed set, and that no
//     name or registry line binds, is listed in a waiver. The
//     waiver count has a ceiling that only moves down.
//  3. Two columns with the same name and the same value set are one
//     vocabulary spelled twice. They bind to one Go type, or the group is
//     listed with the reason it is not.
//
// Sets are compared as sets, never as text: a CHECK that lists the same three
// values in another order holds the same vocabulary.

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// vocabularyCatalog is the schema the migrations build, committed and held
// current by the migration gates. A CHECK read here is the one the database
// enforces, after every later migration widened or narrowed it.
const vocabularyCatalog = "migrations/testdata/head_catalog.txt"

// catalogCheckLine reads one CHECK constraint line of the catalog.
var catalogCheckLine = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.[a-z0-9_]+ CHECK (.*)$`)

// catalogTextColumn reads one text column line of the catalog.
var catalogTextColumn = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*) text\b`)

// wholeVocabularyCheck matches a CHECK whose whole body is the vocabulary,
// optionally behind `col IS NULL OR`. A list inside a wider rule states a
// condition, such as `audience = ANY (…)` only for one kind. It is not the
// column's vocabulary, so it is not read.
var wholeVocabularyCheck = regexp.MustCompile(
	`^\((?:\(\(([a-z_][a-z0-9_]*) IS NULL\) OR )?\(([a-z_][a-z0-9_]*) = ANY \(ARRAY\[([^\]]*)\]\)\)\)?\)$`)

// extensionArm is the open arm an extension-widened CHECK carries beside its
// closed list: the `ext:` spelling an extension kind uses. The closed list is
// the column's vocabulary, so the arm is set aside before the list is read.
var extensionArm = regexp.MustCompile(` OR \([a-z_][a-z0-9_]* ~ '\^ext:[^']*'::text\)`)

// readWholeVocabulary reads a CHECK body that is a whole vocabulary, behind
// an extension arm or not. It returns the submatches of wholeVocabularyCheck,
// or nil when the body is a list inside a wider rule.
func readWholeVocabulary(body string) []string {
	if stripped := extensionArm.ReplaceAllString(body, ""); stripped != body {
		body = stripped
		if v := wholeVocabularyCheck.FindStringSubmatch(body); v != nil {
			return v
		}
		// Dropping the arm leaves one pair of parentheses too many.
		body = strings.TrimSuffix(strings.TrimPrefix(body, "("), ")")
	}
	return wholeVocabularyCheck.FindStringSubmatch(body)
}

// catalogTextLiteral reads one 'value'::text element of a catalog ARRAY.
var catalogTextLiteral = regexp.MustCompile(`'((?:[^']|'')*)'::text`)

// catalogVocabularies maps "table.column" to the sorted, de-duplicated values
// its CHECK admits, for every text column whose CHECK is a whole vocabulary.
var catalogVocabularies = sync.OnceValues(func() (map[string][]string, error) {
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		return nil, err
	}
	textColumns := map[string]bool{}
	found := map[string][]string{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := catalogTextColumn.FindStringSubmatch(line); m != nil {
			textColumns[m[1]+"."+m[2]] = true
			continue
		}
		m := catalogCheckLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		v := readWholeVocabulary(m[2])
		if v == nil || (v[1] != "" && v[1] != v[2]) {
			continue
		}
		var values []string
		for _, lit := range catalogTextLiteral.FindAllStringSubmatch(v[3], -1) {
			values = append(values, strings.ReplaceAll(lit[1], "''", "'"))
		}
		found[m[1]+"."+v[2]] = asSet(values)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for col, values := range found {
		if textColumns[col] {
			out[col] = values
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s yielded no CHECK vocabulary: the catalog format moved", vocabularyCatalog)
	}
	return out, nil
})

// goTypedSetRoots are the trees whose Go constants can mirror a column. The
// generated contract lives under internal/, so generated files are read too.
var goTypedSetRoots = []string{"cmd", "internal", "pkg"}

// goTypedSets maps "pkgDir.TypeName" to the sorted, de-duplicated string
// values of every constant declared with that type outside tests.
var goTypedSets = sync.OnceValues(func() (map[string][]string, error) {
	out := map[string][]string{}
	for _, root := range goTypedSetRoots {
		err := gatekit.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			file, err := gatekit.ParseFile(p, 0)
			if err != nil {
				return err
			}
			collectTypedConsts(out, path.Dir(p), file)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for key, values := range out {
		out[key] = asSet(values)
	}
	return out, nil
})

// collectTypedConsts adds every `const X Type = "value"` in file to out.
func collectTypedConsts(out map[string][]string, dir string, file *ast.File) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			id, ok := vs.Type.(*ast.Ident)
			if !ok {
				continue
			}
			for _, v := range vs.Values {
				lit, ok := v.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if s, err := strconv.Unquote(lit.Value); err == nil {
					key := dir + "." + id.Name
					out[key] = append(out[key], s)
				}
			}
		}
	}
}

// asSet sorts values and drops repeats, so two spellings of one set compare
// equal whatever order they list it in.
func asSet(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

// setKey renders a value set as one comparable string.
func setKey(values []string) string { return strings.Join(values, "\x00") }

// vocabularyCensusInputs loads both sides, failing the test if either is gone.
func vocabularyCensusInputs(t *testing.T) (vocabularies, typed map[string][]string) {
	t.Helper()
	vocabularies, err := catalogVocabularies()
	if err != nil {
		t.Fatal(err)
	}
	typed, err = goTypedSets()
	if err != nil {
		t.Fatal(err)
	}
	return vocabularies, typed
}

// columnTypeName is the Go type name the contract generator gives a column:
// voice_build + status is VoiceBuildStatus.
func columnTypeName(col string) string {
	var b strings.Builder
	for word := range strings.FieldsFuncSeq(col, func(r rune) bool { return r == '.' || r == '_' }) {
		b.WriteString(strings.ToUpper(word[:1]))
		b.WriteString(word[1:])
	}
	return b.String()
}

// namedBindings maps each column to the typed sets named for it, as
// "pkgDir.TypeName" keys.
func namedBindings(vocabularies, typed map[string][]string) map[string][]string {
	byName := map[string][]string{}
	for key := range typed {
		name := key[strings.LastIndex(key, ".")+1:]
		byName[name] = append(byName[name], key)
	}
	out := map[string][]string{}
	for col := range vocabularies {
		if keys := byName[columnTypeName(col)]; len(keys) > 0 {
			slices.Sort(keys)
			out[col] = keys
		}
	}
	return out
}

// columnBindings lists the Go types a column is held to: those named for it
// and those a registry line in enumsync names.
func columnBindings(col string, named map[string][]string) []string {
	out := slices.Clone(named[col])
	if bound, ok := enumBindings[col]; ok {
		out = append(out, bound.pkgDir+"."+bound.typeName)
	}
	for _, request := range requestEnumBindings {
		if request.col == col {
			out = append(out, "internal/contracts."+request.typeName)
		}
	}
	return asSet(out)
}

// namedVocabularyDrift waives a column whose same-named Go type does not hold
// the same set as its CHECK. The reason says which side is wider. The waiver
// goes stale the day the two agree.
var namedVocabularyDrift = gatekit.Waive(map[string]string{
	"attachment.entity_type": "the contract lists six record types and the CHECK admits seventeen; " +
		"an attachment on a deal room or an offer reads back with a type the contract cannot name",
	"capture_connection.provider": "the CHECK also admits offline_demo, telegram and whatsapp, " +
		"which the contract's provider enum does not name",
	"channel_connection.status": "the contract names a pending state the CHECK refuses; no writer stores it, " +
		"the capture store writes only connected, disconnected, error and reauth_required",
	"company_profile_field.source": "the contract shares one source enum with company_fact; technical_lookup " +
		"writes only company_fact, whose CHECK admits it, and never a profile field",
	"custom_field.object": "the contract lists seven targets and the CHECK also admits partner and " +
		"relationship; enumsync binds the column to fieldcatalog.Target instead",
	"finance_connection.status": "the CHECK also admits disconnected, which the contract's status enum " +
		"does not name",
	"forecast_contribution.exclusion_reason": "the CHECK also admits not_eligible and out_of_period, " +
		"which the contract's reason enum does not name",
	"meeting_invitation.status": "the CHECK also admits erased, which the contract's status enum does " +
		"not name",
	"provider_run.subject_kind": "the CHECK also admits scrubbed, the kind a retention scrub writes, " +
		"which the contract does not name",
	"sales_initiative_member.reason": "carried and rule are reasons the membership read derives; the table " +
		"stores only picked and excluded, which is what its CHECK admits",
	"scheduled_send.status": "sent is derived at read from the delivery's own status and never stored, " +
		"so the CHECK does not admit it",
	"signal_resolution.outcome": "the CHECK also admits acknowledged, which the contract's outcome enum " +
		"and the extension's do not name",
	"weekly_review_movement.bar": "the CHECK also admits excluded, not_checked and reinstated, which " +
		"the contract's bar enum does not name",
	"weekly_review_outlook.forward_measure": "the CHECK also admits commit_evidence and " +
		"qualified_commit_evidence, which the contract's measure enum does not name",
})

// namedVocabularyDriftCeiling is how many named pairs may disagree. It only
// moves down: fix a pair, delete its waiver, lower this number.
const namedVocabularyDriftCeiling = 14

func TestEveryColumnNamedGoTypeHoldsItsColumnsCheck(t *testing.T) {
	t.Parallel()
	vocabularies, typed := vocabularyCensusInputs(t)
	named := namedBindings(vocabularies, typed)
	if len(named) < 100 {
		t.Fatalf("only %d columns have a Go type named for them; the derivation broke, so this gate "+
			"would read green over a tree it no longer sees", len(named))
	}
	for _, col := range slices.Sorted(maps.Keys(named)) {
		want := vocabularies[col]
		var drifted []string
		for _, key := range named[col] {
			if setKey(typed[key]) != setKey(want) {
				drifted = append(drifted, fmt.Sprintf("%s %v", key, typed[key]))
			}
		}
		if len(drifted) == 0 || namedVocabularyDrift.Waived(t, col) {
			continue
		}
		t.Errorf("%s: the CHECK admits %v, and the Go type named for this column holds %s. "+
			"A value Go writes and the CHECK refuses fails at insert; a value the CHECK holds and Go "+
			"cannot name reads back as nothing. Change both together",
			col, want, strings.Join(drifted, "; "))
	}
	namedVocabularyDrift.AssertAllMatched(t)
	assertCeiling(t, "namedVocabularyDrift", len(namedVocabularyDrift.Subjects()), namedVocabularyDriftCeiling)
}

// assertCeiling holds a waiver count to a ceiling that only moves down.
func assertCeiling(t *testing.T, name string, count, ceiling int) {
	t.Helper()
	switch {
	case count > ceiling:
		t.Errorf("%s holds %d waivers and its ceiling is %d: bind the new vocabulary instead of waiving it",
			name, count, ceiling)
	case count < ceiling:
		t.Errorf("%s holds %d waivers and its ceiling is still %d: lower the ceiling to %d so the "+
			"count cannot grow back", name, count, ceiling, count)
	}
}

func TestEveryMirroredVocabularyIsBound(t *testing.T) {
	t.Parallel()
	vocabularies, typed := vocabularyCensusInputs(t)
	named := namedBindings(vocabularies, typed)
	bySet := map[string][]string{}
	for key, values := range typed {
		bySet[setKey(values)] = append(bySet[setKey(values)], key)
	}
	matched := 0
	for _, col := range slices.Sorted(maps.Keys(vocabularies)) {
		values := vocabularies[col]
		mirrors := bySet[setKey(values)]
		if len(values) < 2 || len(mirrors) == 0 {
			continue
		}
		matched++
		if len(columnBindings(col, named)) > 0 {
			continue
		}
		if unboundVocabularies.Waived(t, col) {
			continue
		}
		slices.Sort(mirrors)
		t.Errorf("%s: its CHECK set %v is exactly the Go set of %s, and no gate holds the two together. "+
			"Bind the column in enumsync's registry to the type that means it. If the match is "+
			"coincidence, waive the column in unboundVocabularies and say why",
			col, values, strings.Join(mirrors, ", "))
	}
	if matched < 100 {
		t.Fatalf("only %d vocabularies match a Go set; the derivation broke", matched)
	}
	unboundVocabularies.AssertAllMatched(t)
	assertCeiling(t, "unboundVocabularies", len(unboundVocabularies.Subjects()), unboundVocabulariesCeiling)
}

// repeatedVocabulary keys a group of columns that share a name and a value
// set: the column name, then its tables in order. A new table joining the group
// changes the key, so the next copy of a vocabulary is reviewed too.
func repeatedVocabulary(column string, tables []string) string {
	return column + ": " + strings.Join(tables, ", ")
}

func TestARepeatedVocabularyIsBoundToOneGoType(t *testing.T) {
	t.Parallel()
	vocabularies, typed := vocabularyCensusInputs(t)
	named := namedBindings(vocabularies, typed)
	groups := map[string][]string{}
	for col, values := range vocabularies {
		if len(values) < 2 {
			continue
		}
		table, column, _ := strings.Cut(col, ".")
		key := column + "\x01" + setKey(values)
		groups[key] = append(groups[key], table)
	}
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		tables := groups[key]
		if len(tables) < 2 {
			continue
		}
		slices.Sort(tables)
		column, _, _ := strings.Cut(key, "\x01")
		shared := columnBindings(tables[0]+"."+column, named)
		for _, table := range tables[1:] {
			shared = slices.DeleteFunc(shared, func(k string) bool {
				return !slices.Contains(columnBindings(table+"."+column, named), k)
			})
		}
		if len(shared) > 0 {
			continue
		}
		group := repeatedVocabulary(column, tables)
		if repeatedVocabularies.Waived(t, group) {
			continue
		}
		t.Errorf("%s spell the vocabulary %v once each, and no one Go type holds them all. Widening one "+
			"leaves the others behind with nothing to report it. Bind every column to the same type in "+
			"enumsync, or list the group in repeatedVocabularies with the reason the copies are apart",
			group, vocabularies[tables[0]+"."+column])
	}
	repeatedVocabularies.AssertAllMatched(t)
	assertCeiling(t, "repeatedVocabularies", len(repeatedVocabularies.Subjects()), repeatedVocabulariesCeiling)
}

// unboundVocabularies waives a column whose CHECK set equals a Go set by
// coincidence: the types that match name some other concept.
var unboundVocabularies = gatekit.Waive(map[string]string{
	"communication_decision.subject_kind": "four contract enums hold contact and lead, each for a request or " +
		"evidence shape; none names the subject a send decision was made about",
	"deal_room_thread.state": "the only match is ContactCaseStatus, a group of possible duplicate contacts, " +
		"which has nothing to do with a deal room thread",
	"privacy_request_proof.completed_by_type": "the matching enums say who captured a record or a consent " +
		"event; none says who completed a privacy request",
	"runner_job.status": "runner_job is the queue several readers share; each matching enum is one reader's " +
		"own status, and binding the queue to one of them would tie it to that reader",
})

// unboundVocabulariesCeiling only moves down: bind or drop a waived column,
// then lower it.
const unboundVocabulariesCeiling = 4

// sharedByOwnBinding is the reason for a group whose columns each bind by name
// to their own contract enum. A copy that drifts from its enum fails alone.
const sharedByOwnBinding = "each table has its own contract enum and each column is bound to it by name, " +
	"so a copy that drifts from its enum fails on its own"

// repeatedVocabularies waives a group of same-named columns with one value set
// that no single Go type holds, keyed by repeatedVocabulary.
var repeatedVocabularies = gatekit.Waive(map[string]string{
	"bar: weekly_review_driver, weekly_review_movement": "the movement column's contract enum is narrower " +
		"than its CHECK (namedVocabularyDrift), so both cannot bind to it until that drift is settled",
	"execution_authority: comms_outbound, communication_decision": "no Go type spells this vocabulary as " +
		"typed constants, so there is no one type to bind both columns to",
	"forward_measure: forecast_snapshot, team_weekly_review_outlook, weekly_review_outlook": "the outlook " +
		"column's contract enum is narrower than its CHECK (namedVocabularyDrift), so the three cannot " +
		"bind to it until that drift is settled",
	"kind: capture_counterparty_hold, capture_owner_identity, capture_owner_identity_withdrawn": sharedByOwnBinding,
	"entity_type: activity_link_suggestion, signal":                                             sharedByOwnBinding,
	"state: deal_suggestion, tag_suggestion":                                                    sharedByOwnBinding,
	"state: web_form, webhook_subscription":                                                     sharedByOwnBinding,
	"status: assurance_run, close_date_run": "no Go type spells this vocabulary as typed constants, so " +
		"there is no one type to bind both run tables to",
	"status: capture_connection, channel_connection": "channel_connection's contract enum names a pending " +
		"state its CHECK refuses (namedVocabularyDrift), so the two cannot share one type until it is settled",
	"status: attachment_extraction, runner_job, transcript_read": "runner_job is the queue several readers " +
		"share, and each reader's table is bound by name to its own contract status",
})

// repeatedVocabulariesCeiling only moves down: bind a group to one type,
// delete its waiver, then lower it.
const repeatedVocabulariesCeiling = 10

// unreadArrayCheckCeiling is how many catalog CHECKs hold an `= ANY (ARRAY[…])`
// list this census does not read as a column's vocabulary. Each states a
// condition inside a wider rule. The ceiling only moves down.
const unreadArrayCheckCeiling = 28

// TestNoVocabularyCheckHidesFromTheCensus fails when a CHECK with a value
// list stops being read, so a catalog format change cannot shrink the census.
func TestNoVocabularyCheckHidesFromTheCensus(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	var unread []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		m := catalogCheckLine.FindStringSubmatch(line)
		if m == nil || !strings.Contains(m[2], " = ANY (ARRAY[") {
			continue
		}
		if v := readWholeVocabulary(m[2]); v == nil || (v[1] != "" && v[1] != v[2]) {
			unread = append(unread, line)
		}
	}
	if len(unread) > unreadArrayCheckCeiling {
		t.Fatalf("%d CHECKs with a value list are not read as a vocabulary, ceiling %d. "+
			"Teach wholeVocabularyCheck the new form. Raise the ceiling only for a list "+
			"inside a wider rule:\n%s",
			len(unread), unreadArrayCheckCeiling, strings.Join(unread, "\n"))
	}
	if len(unread) < unreadArrayCheckCeiling {
		t.Fatalf("only %d unread value-list CHECKs remain; lower unreadArrayCheckCeiling to %d",
			len(unread), len(unread))
	}
}
