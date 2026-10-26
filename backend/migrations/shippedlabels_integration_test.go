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

// An administrator's relabel is their own words in every language, so it drops
// the key; a write that leaves the label alone keeps it.
func TestARelabelDropsTheShippedLabelKey(t *testing.T) {
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
		var id string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM `+name+` WHERE label_key IS NOT NULL LIMIT 1`).Scan(&id); err != nil {
			t.Fatalf("%s holds no keyed row to relabel: %v", table, err)
		}
		keyAfter := func(set string) *string {
			t.Helper()
			var key *string
			if err := tx.QueryRow(ctx, `UPDATE `+name+` SET `+set+` WHERE id = $1 RETURNING label_key`, id).Scan(&key); err != nil {
				t.Fatalf("updating %s: %v", table, err)
			}
			return key
		}
		if keyAfter(`label = label`) == nil {
			t.Errorf("%s: a write that kept the label dropped its key", table)
		}
		if key := keyAfter(`label = label || ' (ours)'`); key != nil {
			t.Errorf("%s: a relabel kept the key %q, so the new words never show", table, *key)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("rolling back %s: %v", table, err)
		}
	}
}
