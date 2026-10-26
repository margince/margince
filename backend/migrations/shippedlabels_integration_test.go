// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A built-in label names a catalog key the frontend renders it from. The two
// sides hold one list between them, so it is checked both ways here.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const shippedLabelsFile = "../frontend/src/lib/shippedlabels.ts"

// labelKeyTables reads the tables with a label_key column off the schema, so a
// table added later is checked the day it lands.
func labelKeyTables(t *testing.T, conn *pgx.Conn) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT table_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND column_name = 'label_key'
		 ORDER BY table_name`)
	if err != nil {
		t.Fatalf("listing the tables with a label key: %v", err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the tables with a label key: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("no table carries label_key; this gate is reading a schema that is gone")
	}
	return tables
}

func seededLabelKeys(t *testing.T, conn *pgx.Conn) []string {
	t.Helper()
	ctx := context.Background()
	var keys []string
	for _, table := range labelKeyTables(t, conn) {
		rows, err := conn.Query(ctx, `SELECT label_key FROM `+pgx.Identifier{table}.Sanitize()+
			` WHERE label_key IS NOT NULL`)
		if err != nil {
			t.Fatalf("reading %s label keys: %v", table, err)
		}
		found, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("collecting %s label keys: %v", table, err)
		}
		keys = append(keys, found...)
	}
	// A review question keeps its key inside the question, not in a column.
	rows, err := conn.Query(ctx, `
		SELECT q ->> 'label_key'
		  FROM activity_review_template, jsonb_array_elements(questions) AS q
		 WHERE q ? 'label_key'`)
	if err != nil {
		t.Fatalf("reading the review question keys: %v", err)
	}
	questions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collecting the review question keys: %v", err)
	}
	keys = append(keys, questions...)
	slices.Sort(keys)
	return slices.Compact(keys)
}

func listedLabelKeys(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(gatekit.ModuleRoot(t), shippedLabelsFile))
	if err != nil {
		t.Fatalf("reading the frontend's shipped label keys: %v", err)
	}
	list := regexp.MustCompile(`(?s)SHIPPED_LABEL_KEYS = \[(.*?)\] as const`).FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares SHIPPED_LABEL_KEYS as an array literal", shippedLabelsFile)
	}
	var keys []string
	for _, quoted := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(list[1], -1) {
		keys = append(keys, string(quoted[1]))
	}
	slices.Sort(keys)
	return keys
}

func TestEveryShippedLabelKeyIsOneTheFrontendRenders(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	seeded := seededLabelKeys(t, conn)
	listed := listedLabelKeys(t)
	if len(seeded) == 0 {
		t.Fatal("the migrations seed no label key, so nothing below is checked")
	}
	for _, key := range seeded {
		if !slices.Contains(listed, key) {
			t.Errorf("the database names %q, which %s does not list, so it renders in English", key, shippedLabelsFile)
		}
	}
	for _, key := range listed {
		if !slices.Contains(seeded, key) {
			t.Errorf("%s lists %q, which no migration seeds", shippedLabelsFile, key)
		}
	}
}

// An administrator's relabel is their own words in every language, so it hides
// the key; putting the shipped words back shows it again.
func TestARelabelHidesTheShippedLabelKeyUntilTheShippedWordsReturn(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	for _, table := range labelKeyTables(t, conn) {
		name := pgx.Identifier{table}.Sanitize()
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatalf("opening a transaction: %v", err)
		}
		var id, shipped, key string
		if err := tx.QueryRow(ctx, `SELECT id::text, label, label_key FROM `+name+
			` WHERE label_key IS NOT NULL LIMIT 1`).Scan(&id, &shipped, &key); err != nil {
			t.Fatalf("%s holds no keyed row to relabel: %v", table, err)
		}
		keyAfter := func(label string) *string {
			t.Helper()
			var got *string
			if err := tx.QueryRow(ctx, `UPDATE `+name+` SET label = $2 WHERE id = $1 RETURNING label_key`,
				id, label).Scan(&got); err != nil {
				t.Fatalf("relabelling %s: %v", table, err)
			}
			return got
		}
		if got := keyAfter(shipped + " (ours)"); got != nil {
			t.Errorf("%s: a relabel kept the key %q, so the new words never show", table, *got)
		}
		if got := keyAfter(shipped); got == nil || *got != key {
			t.Errorf("%s: the shipped words came back and the key reads %v, want %q", table, got, key)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("rolling back %s: %v", table, err)
		}
	}
}

// A built-in question follows the same rule inside its template, and a save
// cannot hand a question a key of its own choosing.
func TestARewordedReviewQuestionReadsTranslatedOnceItsShippedWordsReturn(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("opening a transaction: %v", err)
	}

	const question = "why_we_won"
	keyAfter := func(label, forged string) *string {
		t.Helper()
		var got *string
		if err := tx.QueryRow(ctx, `
			UPDATE activity_review_template t
			   SET questions = (
			         SELECT jsonb_agg(CASE WHEN q ->> 'key' = $1
			                               THEN q || jsonb_build_object('label', $2::text, 'label_key', $3::text)
			                               ELSE q END ORDER BY ord)
			           FROM jsonb_array_elements(t.questions) WITH ORDINALITY AS a (q, ord))
			 WHERE t.system AND t.key = 'win_review'
			RETURNING (SELECT q ->> 'label_key' FROM jsonb_array_elements(questions) AS q WHERE q ->> 'key' = $1)`,
			question, label, forged).Scan(&got); err != nil {
			t.Fatalf("saving the win review: %v", err)
		}
		return got
	}
	const shipped, key = "Why did we win?", "reviewTemplates.builtin.question.why_we_won"
	if got := keyAfter("What tipped it our way?", key); got != nil {
		t.Errorf("a reworded question kept the key %q, so the new words never show", *got)
	}
	if got := keyAfter(shipped, "reviewTemplates.builtin.question.lost_to"); got == nil || *got != key {
		t.Errorf("the shipped words came back and the question's key reads %v, want %q", got, key)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rolling back the win review: %v", err)
	}
}
