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
	"regexp"
	"slices"
	"strings"
	"testing"
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
type foreignKey struct {
	column, references  string
	cascades, validated bool
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

	id := regexp.QuoteMeta(f.pair.id)
	shape := regexp.MustCompile(`^CHECK \(\((?:\(` + id + ` IS NULL\) OR \(|(\w+) OR \()?num_nonnulls\(([^()]*)\) = 1\)\)?\)$`)
	slices.Sort(columns)
	for _, def := range f.checks {
		m := shape.FindStringSubmatch(def)
		if m == nil || (m[1] != "" && !slices.Contains(guarded, m[1])) {
			continue
		}
		read := strings.Split(m[2], ", ")
		slices.Sort(read)
		if slices.Equal(read, columns) {
			return true
		}
	}
	return false
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
	vocabulary := regexp.MustCompile(`^CHECK \(\(` + disc + ` = ANY \(ARRAY\[(.*)\]\)\)\)$`)
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
// pair. A binding CHECK whose marker no trigger guards fails the whole answer,
// since it would excuse new rows too.
//
// A plain key counts as keyed only through a validated cascading foreign key to
// the table its value names. Its rows are what deletes reach, and a key that
// pointed elsewhere or refused nothing would pass the census on its name alone.
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
			return fk.column == m[2] && fk.references == value[1] && fk.cascades && fk.validated
		})
		keys = append(keys, branchKey{column: m[2], expression: m[3], keyed: keyed})
	}
	return keys, true
}

// markerReset is how a trigger function must open to guard a marker. An insert
// clears it before anything else runs. Every other event keeps the old value.
var markerReset = regexp.MustCompile(
	`(?s)^\s*BEGIN\s+IF TG_OP = 'INSERT' THEN\s+NEW\.(\w+) := false;\s+ELSE\s+NEW\.(\w+) := OLD\.(\w+);\s+END IF;`)

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
		if m == nil || !fires || m[1] != m[2] || m[1] != m[3] {
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
		foreignKeys: []foreignKey{{column: "cycle_id", references: "assurance_cycle", cascades: true, validated: true}},
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
			foreignKey{column: column, references: kind, cascades: true, validated: true})
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
		"a key's foreign key was never validated": func(f *shapeFacts) { offerKey(f).validated = false },
		"a key's foreign key does not cascade":    func(f *shapeFacts) { offerKey(f).cascades = false },
		"a key's foreign key names another table": func(f *shapeFacts) { offerKey(f).references = "deal" },
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
