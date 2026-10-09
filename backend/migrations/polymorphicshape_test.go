// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations_test

// When a polymorphic pair that keeps its id counts as answered by SHAPE.
//
// Read off the catalog text Postgres prints, so planted schemas can test the
// judgement without a database. It needs a key column for each value the
// discriminator admits, each with a foreign key. It also needs a CHECK that holds
// those columns, and no others, to one value between them.

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

type shapeFacts struct {
	pair   polymorphicPair
	checks []string
	keys   []branchKey
}

var quotedValue = regexp.MustCompile(`'([^']*)'::text`)

// shapeAnswers says whether the generated keys cover every value the
// discriminator admits and each carries a foreign key. One CHECK must hold those
// keys, and no others, to a single value per row.
func shapeAnswers(f shapeFacts) bool {
	disc, id := regexp.QuoteMeta(f.pair.discriminator), regexp.QuoteMeta(f.pair.id)
	vocabulary := regexp.MustCompile(`^CHECK \(\(` + disc + ` = ANY \(ARRAY\[(.*)\]\)\)\)$`)
	// A value whose rows carry no id, stated by the schema: (kind = 'v') = (id IS NULL).
	idless := regexp.MustCompile(`^CHECK \(\(\(` + disc + ` = '([^']*)'::text\) = \(` + id + ` IS NULL\)\)\)$`)
	branch := regexp.MustCompile(`^CASE\s+WHEN \(` + disc + ` = '([^']*)'::text\) THEN ` + id + `\s+ELSE NULL::uuid\s+END$`)
	shape := regexp.MustCompile(`^CHECK \(\((?:\(` + id + ` IS NULL\) OR \()?num_nonnulls\(([^()]*)\) = 1\)\)?\)$`)

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
	if len(owed) == 0 {
		return false
	}

	var covered, columns []string
	for _, key := range f.keys {
		m := branch.FindStringSubmatch(strings.TrimSpace(key.expression))
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
		m := shape.FindStringSubmatch(def)
		if m == nil {
			continue
		}
		read := strings.Split(m[1], ", ")
		slices.Sort(read)
		if slices.Equal(read, columns) {
			return true
		}
	}
	return false
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
