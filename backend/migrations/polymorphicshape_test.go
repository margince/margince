// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations_test

// When a polymorphic pair that keeps its id counts as answered by SHAPE.
//
// Read off the catalog text Postgres prints, so planted schemas can test the
// judgement without a database. It needs a key column for each value the
// discriminator admits, each with a foreign key. It also needs a CHECK that holds
// those columns, and no others, to one value between them.
//
// A stored generated key is bound to the pair by its expression. A plain key a
// trigger fills is bound by a CHECK that it equals that expression. The plain
// form is for a table that must not be rewritten. Its older rows carry a marker
// that excuses them from the CHECKs. The marker counts only when a trigger clears
// it on every insert and keeps it on every update.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type polymorphicPair struct{ table, id, discriminator string }

// branchKey is one stored generated column built from the pair's id.
type branchKey struct {
	column, expression string
	keyed              bool
}

// rowTrigger is one `BEFORE ... FOR EACH ROW` trigger on the table, with the
// source of the function it runs. enabled is pg_trigger.tgenabled. qualified is a
// `WHEN` clause and columns an `UPDATE OF` list: either lets a row past it.
type rowTrigger struct {
	onInsert, onUpdate, qualified, columns bool
	enabled, source                        string
}

// foreignKey is one single-column foreign key, with what a plain key needs of it.
// onDelete is pg_constraint.confdeltype: c for CASCADE, n for SET NULL.
type foreignKey struct {
	column, references, onDelete string
	validated                    bool
}

type shapeFacts struct {
	pair        polymorphicPair
	checks      []string
	keys        []branchKey
	foreignKeys []foreignKey
	// flags are the NOT NULL boolean columns, the only ones a marker can be.
	flags    []string
	triggers []rowTrigger
}

var quotedValue = regexp.MustCompile(`'([^']*)'::text`)

// shapeAnswers says whether the keys cover every value the discriminator admits
// and each carries a foreign key. One CHECK must hold those keys, and no others,
// to a single value per row.
func shapeAnswers(f shapeFacts) bool {
	owed := owedValues(f)
	if len(owed) == 0 {
		return false
	}
	guarded := guardedMarkers(f)
	keys, ok := boundKeys(f, guarded)
	if !ok {
		return false
	}

	var covered, columns []string
	for _, key := range keys {
		m := branchExpression(f.pair).FindStringSubmatch(strings.TrimSpace(key.expression))
		if m == nil || !key.keyed || slices.Contains(covered, m[1]) {
			return false
		}
		covered = append(covered, m[1])
		columns = append(columns, key.column)
	}
	slices.Sort(owed)
	slices.Sort(covered)
	if !slices.Equal(owed, covered) {
		return false
	}

	slices.Sort(columns)
	for _, def := range f.checks {
		marker, keyList, ok := shapeCheck(f.pair, def)
		if !ok || (marker != "" && !slices.Contains(guarded, marker)) {
			continue
		}
		read := strings.Split(keyList, ", ")
		slices.Sort(read)
		if slices.Equal(read, columns) {
			return true
		}
	}
	return false
}

// shapeCheck reads a CHECK that holds the keys to one value per row. It may be
// excused by a marker, by an absent id, or by both, in that order.
func shapeCheck(pair polymorphicPair, def string) (marker, keys string, ok bool) {
	id := regexp.QuoteMeta(pair.id)
	shape := regexp.MustCompile(`^CHECK \(\((?:(?:(\w+) OR )?(?:\(` + id + ` IS NULL\) OR )?` +
		`\(num_nonnulls\(([^()]*)\) = 1\)|num_nonnulls\(([^()]*)\) = 1)\)\)$`)
	m := shape.FindStringSubmatch(def)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2] + m[3], true
}

// branchExpression matches the expression that derives one key from the pair:
// the id when the discriminator names the key's value, and NULL otherwise.
func branchExpression(pair polymorphicPair) *regexp.Regexp {
	disc, id := regexp.QuoteMeta(pair.discriminator), regexp.QuoteMeta(pair.id)
	return regexp.MustCompile(`^CASE\s+WHEN \(` + disc + ` = '([^']*)'::text\) THEN ` + id + `\s+ELSE NULL::uuid\s+END$`)
}

// owedValues are the discriminator values whose rows carry an id, each of which
// owes a key column.
func owedValues(f shapeFacts) []string {
	disc, id := regexp.QuoteMeta(f.pair.discriminator), regexp.QuoteMeta(f.pair.id)
	// The discriminator may be NULL where the id is: a row that names no record.
	vocabulary := regexp.MustCompile(`^CHECK \(\((?:\(` + disc + ` IS NULL\) OR \()?` +
		disc + ` = ANY \(ARRAY\[(.*)\]\)\)\)\)?$`)
	// A value whose rows carry no id, stated by the schema: (kind = 'v') = (id IS NULL).
	idless := regexp.MustCompile(`^CHECK \(\(\(` + disc + ` = '([^']*)'::text\) = \(` + id + ` IS NULL\)\)\)$`)

	var admitted, withoutID []string
	for _, def := range f.checks {
		if m := vocabulary.FindStringSubmatch(def); m != nil {
			for _, v := range quotedValue.FindAllStringSubmatch(m[1], -1) {
				admitted = append(admitted, v[1])
			}
		}
		if m := idless.FindStringSubmatch(def); m != nil {
			withoutID = append(withoutID, m[1])
		}
	}
	var owed []string
	for _, v := range admitted {
		if !slices.Contains(withoutID, v) {
			owed = append(owed, v)
		}
	}
	return owed
}

// boundKeys are the generated keys plus every plain column a CHECK binds to the
// pair. A binding CHECK with an unguarded marker fails the whole answer.
// A plain key counts only through a validated foreign key to the table its
// value names, which cascades or sets NULL. SET NULL counts only beside a
// trigger that clears the pair. The action's own update is not checked against
// the key, so a trigger that refilled it would leave it naming the gone row.
func boundKeys(f shapeFacts, guarded []string) ([]branchKey, bool) {
	binding := regexp.MustCompile(`(?s)^CHECK \(\((\w+) OR \(NOT \((\w+) IS DISTINCT FROM\s+(CASE.*END)\)\)\)\)$`)
	keys := slices.Clone(f.keys)
	for _, def := range f.checks {
		m := binding.FindStringSubmatch(def)
		if m == nil {
			continue
		}
		if !slices.Contains(guarded, m[1]) {
			return nil, false
		}
		value := branchExpression(f.pair).FindStringSubmatch(strings.TrimSpace(m[3]))
		keyed := value != nil && slices.ContainsFunc(f.foreignKeys, func(fk foreignKey) bool {
			follows := fk.onDelete == "c" || (fk.onDelete == "n" && clearsPair(f))
			return fk.column == m[2] && fk.references == value[1] && follows && fk.validated
		})
		keys = append(keys, branchKey{column: m[2], expression: m[3], keyed: keyed})
	}
	return keys, true
}

// clearsPair says whether an update trigger sets both halves of the pair to
// NULL. That is how a row lets go of a record its SET NULL key cleared.
func clearsPair(f shapeFacts) bool {
	disc, id := regexp.QuoteMeta(f.pair.discriminator), regexp.QuoteMeta(f.pair.id)
	clearsDisc := regexp.MustCompile(`\bNEW\.` + disc + ` := NULL;`)
	clearsID := regexp.MustCompile(`\bNEW\.` + id + ` := NULL;`)
	return slices.ContainsFunc(f.triggers, func(trigger rowTrigger) bool {
		return trigger.onUpdate && (trigger.enabled == "O" || trigger.enabled == "A") &&
			clearsDisc.MatchString(trigger.source) && clearsID.MatchString(trigger.source)
	})
}

// markerReset is how a trigger function must open to guard a marker. An insert
// clears it before anything else runs. Every other event keeps the old value,
// or clears it when the update asks: `OLD.m AND NEW.m` can never set it.
var markerReset = regexp.MustCompile(
	`(?s)^\s*BEGIN\s+IF TG_OP = 'INSERT' THEN\s+NEW\.(\w+) := false;\s+ELSE\s+NEW\.(\w+) := OLD\.(\w+)(?: AND NEW\.(\w+))?;\s+END IF;`)

// guardedMarkers are the NOT NULL boolean columns a writer cannot set. One
// trigger opens with markerReset for it and fires on every insert and update.
// It is enabled always or on origin, with no `WHEN` clause and no column list.
// No trigger function assigns the marker anywhere else.
func guardedMarkers(f shapeFacts) []string {
	var guarded []string
	for _, trigger := range f.triggers {
		m := markerReset.FindStringSubmatch(trigger.source)
		fires := trigger.onInsert && trigger.onUpdate && !trigger.qualified && !trigger.columns &&
			(trigger.enabled == "O" || trigger.enabled == "A")
		if m == nil || !fires || m[1] != m[2] || m[1] != m[3] || (m[4] != "" && m[4] != m[1]) {
			continue
		}
		if slices.Contains(f.flags, m[1]) && !assignedElsewhere(f.triggers, m[1]) {
			guarded = append(guarded, m[1])
		}
	}
	return guarded
}

// assignedElsewhere says whether any trigger function assigns the marker
// beyond the two assignments of the reset itself. PL/pgSQL assigns with `:=`,
// `=` and `SELECT ... INTO`, in any case. A comparison also matches, which only
// errs towards refusing.
func assignedElsewhere(triggers []rowTrigger, marker string) bool {
	name := regexp.QuoteMeta(marker)
	assignment := regexp.MustCompile(`(?i)\bNEW\.` + name + `\s*:?=|\bINTO\b[^;]*\bNEW\.` + name + `\b`)
	assignments := 0
	for _, trigger := range triggers {
		assignments += len(assignment.FindAllString(trigger.source, -1))
	}
	return assignments != 2
}

// recordGrantShape is the shape record_grant ships, as the catalog prints it.
func recordGrantShape() shapeFacts {
	types := []string{"contact", "company", "deal", "lead", "project"}
	f := shapeFacts{
		pair: polymorphicPair{table: "record_grant", id: "record_id", discriminator: "record_type"},
		checks: []string{
			"CHECK ((record_type = ANY (ARRAY['contact'::text, 'company'::text, 'deal'::text, 'lead'::text, 'project'::text])))",
			"CHECK ((num_nonnulls(contact_id, company_id, deal_id, lead_id, project_id) = 1))",
		},
	}
	for _, kind := range types {
		f.keys = append(f.keys, branchKey{
			column:     kind + "_id",
			expression: "CASE\n    WHEN (record_type = '" + kind + "'::text) THEN record_id\n    ELSE NULL::uuid\nEND",
			keyed:      true,
		})
	}
	return f
}

func TestAShapedPairIsAnsweredOnlyWhenEveryBranchIsKeyedAndBound(t *testing.T) {
	if !shapeAnswers(recordGrantShape()) {
		t.Fatal("the shape record_grant ships was not recognised; every case below would pass vacuously")
	}

	cases := map[string]func(*shapeFacts){
		"a value the discriminator admits has no key column": func(f *shapeFacts) {
			f.keys = f.keys[:len(f.keys)-1]
			f.checks[1] = "CHECK ((num_nonnulls(contact_id, company_id, deal_id, lead_id) = 1))"
		},
		"a key column has no foreign key": func(f *shapeFacts) { f.keys[2].keyed = false },
		"the CHECK holds nothing": func(f *shapeFacts) {
			f.checks[1] = "CHECK ((num_nonnulls(contact_id, company_id, deal_id, lead_id, project_id) >= 0))"
		},
		"the CHECK reads only some of the keys": func(f *shapeFacts) {
			f.checks[1] = "CHECK ((num_nonnulls(contact_id, company_id) = 1))"
		},
		"there is no CHECK": func(f *shapeFacts) { f.checks = f.checks[:1] },
		"a key column is built from another value twice": func(f *shapeFacts) {
			f.keys[1].expression = f.keys[0].expression
		},
		"the discriminator admits nothing the schema states": func(f *shapeFacts) { f.checks = f.checks[1:] },
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			f := recordGrantShape()
			plant(&f)
			if shapeAnswers(f) {
				t.Error("the pair was counted as answered, so it would leave the census unresolved")
			}
		})
	}
}

func TestAValueWithoutAnIDOwesNoKeyColumn(t *testing.T) {
	f := shapeFacts{
		pair: polymorphicPair{table: "analytics_share", id: "scope_id", discriminator: "scope_kind"},
		checks: []string{
			"CHECK ((scope_kind = ANY (ARRAY['workspace'::text, 'team'::text, 'owner'::text])))",
			"CHECK (((scope_kind = 'workspace'::text) = (scope_id IS NULL)))",
			"CHECK (((scope_id IS NULL) OR (num_nonnulls(scope_team_id, scope_user_id) = 1)))",
		},
		keys: []branchKey{
			{column: "scope_team_id", expression: "CASE WHEN (scope_kind = 'team'::text) THEN scope_id ELSE NULL::uuid END", keyed: true},
			{column: "scope_user_id", expression: "CASE WHEN (scope_kind = 'owner'::text) THEN scope_id ELSE NULL::uuid END", keyed: true},
		},
	}
	if !shapeAnswers(f) {
		t.Fatal("a workspace scope the schema states has no id was treated as a missing branch")
	}
	f.checks = slices.Delete(f.checks, 1, 2)
	if shapeAnswers(f) {
		t.Error("without the CHECK that says the workspace has no id, the workspace value " +
			"is a branch with no key column, and the pair was still counted as answered")
	}
}

// taskItemShape is the shape assurance_task_item ships, as the catalog prints
// it: plain keys a trigger fills, bound by CHECKs a marker excuses.
func taskItemShape() shapeFacts {
	kinds := []string{"deal", "signal", "offer", "contract"}
	f := shapeFacts{
		pair: polymorphicPair{table: "assurance_task_item", id: "subject_id", discriminator: "subject_kind"},
		checks: []string{
			"CHECK ((subject_kind = ANY (ARRAY['deal'::text, 'signal'::text, 'offer'::text, 'contract'::text])))",
			"CHECK ((subject_unkeyed OR (num_nonnulls(subject_deal_id, subject_signal_id, subject_offer_id, subject_contract_id) = 1)))",
		},
		foreignKeys: []foreignKey{{column: "cycle_id", references: "assurance_cycle", onDelete: "c", validated: true}},
		flags:       []string{"subject_unkeyed"},
		triggers: []rowTrigger{
			{onInsert: true, onUpdate: true, enabled: "O", source: taskItemKeysSource},
			{onUpdate: true, enabled: "O", source: "\nBEGIN\n  NEW.updated_at := now();\n  RETURN NEW;\nEND"},
		},
	}
	for _, kind := range kinds {
		column := "subject_" + kind + "_id"
		f.checks = append(f.checks, "CHECK ((subject_unkeyed OR (NOT ("+column+" IS DISTINCT FROM\nCASE\n"+
			"    WHEN (subject_kind = '"+kind+"'::text) THEN subject_id\n    ELSE NULL::uuid\nEND))))")
		f.foreignKeys = append(f.foreignKeys,
			foreignKey{column: column, references: kind, onDelete: "c", validated: true})
	}
	return f
}

const taskItemKeysSource = `
BEGIN
	IF TG_OP = 'INSERT' THEN
		NEW.subject_unkeyed := false;
	ELSE
		NEW.subject_unkeyed := OLD.subject_unkeyed;
	END IF;
	IF NOT NEW.subject_unkeyed THEN
		NEW.subject_deal_id := CASE WHEN NEW.subject_kind = 'deal' THEN NEW.subject_id END;
	END IF;
	RETURN NEW;
END `

// offerKey is the foreign key of the offer branch, which the cases below bend.
func offerKey(f *shapeFacts) *foreignKey {
	for i := range f.foreignKeys {
		if f.foreignKeys[i].column == "subject_offer_id" {
			return &f.foreignKeys[i]
		}
	}
	panic("the fixture has no offer key")
}

func TestATriggerFilledShapeIsAnsweredOnlyWhenTheMarkerIsGuarded(t *testing.T) {
	if !shapeAnswers(taskItemShape()) {
		t.Fatal("the shape assurance_task_item ships was not recognised; every case below would pass vacuously")
	}

	cases := map[string]func(*shapeFacts){
		"no trigger clears the marker": func(f *shapeFacts) { f.triggers = f.triggers[1:] },
		"the trigger does not fire on update, so an update can set the marker": func(f *shapeFacts) {
			f.triggers[0].onUpdate = false
		},
		"the trigger does not fire on insert, so a writer can set the marker": func(f *shapeFacts) {
			f.triggers[0].onInsert = false
		},
		"the trigger is disabled":                  func(f *shapeFacts) { f.triggers[0].enabled = "D" },
		"the trigger fires only on a replica":      func(f *shapeFacts) { f.triggers[0].enabled = "R" },
		"a WHEN clause lets rows past the trigger": func(f *shapeFacts) { f.triggers[0].qualified = true },
		"an UPDATE OF list lets updates past the trigger": func(f *shapeFacts) {
			f.triggers[0].columns = true
		},
		"the trigger clears the marker only after other work": func(f *shapeFacts) {
			f.triggers[0].source = "\nBEGIN\n\tPERFORM 1;" + strings.TrimPrefix(f.triggers[0].source, "\nBEGIN")
		},
		"the trigger sets the marker again with :=": func(f *shapeFacts) {
			f.triggers[0].source = strings.Replace(f.triggers[0].source,
				"RETURN NEW;", "NEW.subject_unkeyed := true;\n\tRETURN NEW;", 1)
		},
		"the trigger sets the marker again with =": func(f *shapeFacts) {
			f.triggers[0].source = strings.Replace(f.triggers[0].source,
				"RETURN NEW;", "new.subject_unkeyed = true;\n\tRETURN NEW;", 1)
		},
		"the trigger sets the marker again with SELECT INTO": func(f *shapeFacts) {
			f.triggers[0].source = strings.Replace(f.triggers[0].source,
				"RETURN NEW;", "SELECT true INTO NEW.subject_unkeyed;\n\tRETURN NEW;", 1)
		},
		"another trigger sets the marker": func(f *shapeFacts) {
			f.triggers[1].source = "\nBEGIN\n  NEW.subject_unkeyed := true;\n  RETURN NEW;\nEND"
		},
		"the marker may be NULL, which a CHECK admits": func(f *shapeFacts) { f.flags = nil },
		"the shape CHECK is excused by a column no trigger guards": func(f *shapeFacts) {
			f.checks[1] = strings.Replace(f.checks[1], "subject_unkeyed", "archived", 1)
		},
		"a binding CHECK is excused by a column no trigger guards": func(f *shapeFacts) {
			f.checks[2] = strings.Replace(f.checks[2], "subject_unkeyed", "archived", 1)
		},
		"a key column has no binding CHECK": func(f *shapeFacts) { f.checks = f.checks[:5] },
		"a key column has no foreign key": func(f *shapeFacts) {
			f.foreignKeys = slices.DeleteFunc(f.foreignKeys,
				func(fk foreignKey) bool { return fk.column == "subject_offer_id" })
		},
		"a key's foreign key was never validated":        func(f *shapeFacts) { offerKey(f).validated = false },
		"a key's foreign key does not follow the delete": func(f *shapeFacts) { offerKey(f).onDelete = "a" },
		"a key sets NULL and nothing clears the pair":    func(f *shapeFacts) { offerKey(f).onDelete = "n" },
		"a key's foreign key names another table":        func(f *shapeFacts) { offerKey(f).references = "deal" },
		"two key columns are bound to one value": func(f *shapeFacts) {
			f.checks[5] = strings.Replace(f.checks[5], "'contract'", "'deal'", 1)
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			f := taskItemShape()
			plant(&f)
			if shapeAnswers(f) {
				t.Error("the pair was counted as answered, so it would leave the census unresolved")
			}
		})
	}
}

// commitmentShape is the shape weekly_plan_commitment ships. Its pair may name
// no record, its keys let go of a deleted record, and an update may clear its
// marker.
func commitmentShape() shapeFacts {
	types := []string{"deal", "lead", "contact", "company", "project"}
	f := shapeFacts{
		pair: polymorphicPair{table: "weekly_plan_commitment", id: "linked_record_id", discriminator: "linked_record_type"},
		checks: []string{
			"CHECK (((linked_record_type IS NULL) OR (linked_record_type = ANY (ARRAY['deal'::text, 'lead'::text, " +
				"'contact'::text, 'company'::text, 'project'::text]))))",
			"CHECK ((linked_record_unkeyed OR (linked_record_id IS NULL) OR (num_nonnulls(linked_deal_id, " +
				"linked_lead_id, linked_contact_id, linked_company_id, linked_project_id) = 1)))",
		},
		flags: []string{"linked_record_unkeyed"},
		triggers: []rowTrigger{{onInsert: true, onUpdate: true, enabled: "O", source: `
BEGIN
	IF TG_OP = 'INSERT' THEN
		NEW.linked_record_unkeyed := false;
	ELSE
		NEW.linked_record_unkeyed := OLD.linked_record_unkeyed AND NEW.linked_record_unkeyed;
	END IF;
	IF NOT NEW.linked_record_unkeyed AND NEW.linked_deal_id IS NULL THEN
		NEW.linked_record_type := NULL;
		NEW.linked_record_id := NULL;
	END IF;
	RETURN NEW;
END `}},
	}
	for _, kind := range types {
		column := "linked_" + kind + "_id"
		f.checks = append(f.checks, "CHECK ((linked_record_unkeyed OR (NOT ("+column+" IS DISTINCT FROM\nCASE\n"+
			"    WHEN (linked_record_type = '"+kind+"'::text) THEN linked_record_id\n    ELSE NULL::uuid\nEND))))")
		f.foreignKeys = append(f.foreignKeys,
			foreignKey{column: column, references: kind, onDelete: "n", validated: true})
	}
	return f
}

func TestAShapeOverANullablePairWithAClearableMarkerIsAnswered(t *testing.T) {
	if !shapeAnswers(commitmentShape()) {
		t.Fatal("the shape weekly_plan_commitment ships was not recognised; every case below would pass vacuously")
	}

	reset := "NEW.linked_record_unkeyed := OLD.linked_record_unkeyed AND NEW.linked_record_unkeyed;"
	cases := map[string]func(*shapeFacts){
		"an update can set the marker": func(f *shapeFacts) {
			f.triggers[0].source = strings.Replace(f.triggers[0].source, reset,
				"NEW.linked_record_unkeyed := OLD.linked_record_unkeyed OR NEW.linked_record_unkeyed;", 1)
		},
		"the update keeps another column's value": func(f *shapeFacts) {
			f.triggers[0].source = strings.Replace(f.triggers[0].source, reset,
				"NEW.linked_record_unkeyed := OLD.linked_record_unkeyed AND NEW.archived;", 1)
		},
		"the shape CHECK names a marker and no pair": func(f *shapeFacts) {
			f.checks[1] = strings.Replace(f.checks[1], "linked_record_unkeyed OR ", "", 1)
			f.checks[1] = strings.Replace(f.checks[1], "(linked_record_id IS NULL)", "(label IS NULL)", 1)
		},
		"no trigger clears the pair a SET NULL key let go of": func(f *shapeFacts) {
			f.triggers[0].source = strings.Replace(f.triggers[0].source, "NEW.linked_record_id := NULL;", "", 1)
		},
		"the vocabulary is excused by another column": func(f *shapeFacts) {
			f.checks[0] = strings.Replace(f.checks[0], "(linked_record_type IS NULL)", "(label IS NULL)", 1)
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			f := commitmentShape()
			plant(&f)
			if shapeAnswers(f) {
				t.Error("the pair was counted as answered, so it would leave the census unresolved")
			}
		})
	}
}

// resolutionGroups are the four endings a register entry may name. A reason
// opens with one of them, so no pair can stand in two groups.
var resolutionGroups = []string{"SHAPE", "SPLIT", "DECIDE", "OUTLIVES"}

// namesOneGroup says whether a register reason opens with one of the groups.
func namesOneGroup(reason string) bool {
	group, _, ok := strings.Cut(reason, " — ")
	return ok && slices.Contains(resolutionGroups, group)
}

func TestARegisterReasonNamesExactlyOneGroup(t *testing.T) {
	for reason, want := range map[string]bool{
		"SHAPE — one key per branch":        true,
		"OUTLIVES — the row is history":     true,
		"SHAPE/OUTLIVES — one or the other": false,
		"shape — lower case is no group":    false,
		"a reason with no group at all":     false,
	} {
		if got := namesOneGroup(reason); got != want {
			t.Errorf("namesOneGroup(%q) = %v, want %v", reason, got, want)
		}
	}
}

// judgePairs holds a register against the unanswered pairs. A pair with no
// entry is a finding, and so is an entry that names no group or two. The
// caller's AssertAllMatched reports an entry whose pair is answered or gone.
func judgePairs(t testing.TB, found []string, register *gatekit.Waivers[string]) {
	t.Helper()
	for _, pair := range found {
		if register.Waived(t, pair) {
			continue
		}
		t.Errorf("%s names a record with nothing to say when that record goes.\n"+
			"Give it one of the three endings — a foreign key per branch (SHAPE), a table per "+
			"branch (SPLIT), or a reason it outlives the record (OUTLIVES) — and put it in "+
			"pairResolution with which and why.", pair)
	}
	for pair, reason := range register.Reasons() {
		if !namesOneGroup(reason) {
			t.Errorf("%s does not open with one of SHAPE, SPLIT, DECIDE or OUTLIVES, so the next "+
				"change cannot tell which work it owes", pair)
		}
	}
}

// findingRecorder keeps the gate's findings instead of failing with them.
type findingRecorder struct {
	testing.TB
	findings []string
}

func (r *findingRecorder) Helper() {}

func (r *findingRecorder) Errorf(format string, args ...any) {
	r.findings = append(r.findings, fmt.Sprintf(format, args...))
}

func TestTheRegisterFailsAPairOutsideEveryGroupAndAStaleEntry(t *testing.T) {
	const fixture, stale = "poly_fixture.subject_id (subject_type)", "gone_table.subject_id (subject_type)"
	cases := map[string]struct {
		found    []string
		register map[string]string
	}{
		"a pair with no entry": {found: []string{fixture}, register: map[string]string{}},
		"an entry whose pair is answered": {register: map[string]string{
			stale: "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE",
		}},
		"an entry that names no group": {found: []string{fixture}, register: map[string]string{
			fixture: "one nullable foreign key per branch, or a table per branch",
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			recorder := &findingRecorder{TB: t}
			register := gatekit.Waive(c.register)
			judgePairs(recorder, c.found, register)
			register.AssertAllMatched(recorder)
			if len(recorder.findings) != 1 {
				t.Errorf("got %d findings, want 1: %q", len(recorder.findings), recorder.findings)
			}
		})
	}
	recorder := &findingRecorder{TB: t}
	register := gatekit.Waive(map[string]string{
		fixture: "SPLIT — narrow table, one table per branch costs less than the columns",
	})
	judgePairs(recorder, []string{fixture}, register)
	register.AssertAllMatched(recorder)
	if len(recorder.findings) != 0 {
		t.Errorf("a classified pair was reported: %q", recorder.findings)
	}
}
