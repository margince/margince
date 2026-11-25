// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	settingsNameOnceUp   = "core/1795573384_a_settings_list_holds_each_name_once.up.sql"
	settingsNameOnceDown = "core/1795573384_a_settings_list_holds_each_name_once.down.sql"
	leadNameBoundUp      = "core/1795573385_a_lead_name_holds_at_most_a_label.up.sql"
	leadNameBoundDown    = "core/1795573385_a_lead_name_holds_at_most_a_label.down.sql"
)

// settingsList is one list the migration holds to a name once. release is what
// a user does to take an entry out of use: retire it, or archive it.
type settingsList struct {
	table, column, index, insert, release string
	releasesName                          bool
}

func settingsLists(pipelineID string) []settingsList {
	retire := func(table string) string { return `UPDATE ` + table + ` SET active = false WHERE id = $1` }
	archive := func(table string) string { return `UPDATE ` + table + ` SET archived_at = now() WHERE id = $1` }
	return []settingsList{
		{
			"deal_acquisition_source", "label", "deal_acquisition_source_label_once",
			`INSERT INTO deal_acquisition_source (key, label, created_at) VALUES ('twin_' || @n::int, @name, @created_at)`,
			retire("deal_acquisition_source"), false,
		},
		{
			"deal_loss_reason", "label", "deal_loss_reason_label_once",
			`INSERT INTO deal_loss_reason (key, label, created_at) VALUES ('twin_' || @n::int, @name, @created_at)`,
			retire("deal_loss_reason"), false,
		},
		{
			"partner_stage", "label", "partner_stage_label_once",
			`INSERT INTO partner_stage (key, label, meaning, created_at) VALUES ('twin_' || @n::int, @name, 'candidate', @created_at)`,
			retire("partner_stage"), false,
		},
		{
			"record_role", "label", "record_role_label_once",
			`INSERT INTO record_role (key, label, assignee_kinds, record_types, created_at)
			 VALUES ('twin_' || @n::int, @name, '{user}', '{deal}', @created_at)`,
			retire("record_role"), false,
		},
		{
			"lead_origin", "label", "lead_origin_label_once",
			`INSERT INTO lead_origin (key, label, created_at) VALUES ('twin_' || @n::int, @name, @created_at)`,
			retire("lead_origin"), false,
		},
		{
			"team", "name", "team_name_unique",
			`INSERT INTO team (name, created_at) VALUES (@name, @created_at)`,
			archive("team"), true,
		},
		{
			"pipeline", "name", "pipeline_name_unique",
			`INSERT INTO pipeline (name, created_at) VALUES (@name, @created_at)`,
			archive("pipeline"), true,
		},
		{
			"stage", "name", "stage_name_once",
			`INSERT INTO stage (pipeline_id, name, position, created_at) VALUES ('` + pipelineID + `', @name, @n, @created_at)`,
			archive("stage"), true,
		},
		{
			"knowledge_corpus", "name", "knowledge_corpus_name_once",
			`INSERT INTO knowledge_corpus (name, topic_statement, captured_by, created_at) VALUES (@name, 'Twins', 'test', @created_at)`,
			archive("knowledge_corpus"), true,
		},
	}
}

func TestSettingsListTwinsAreRenamedBeforeEachNameIsHeldOnce(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	applyMigrationFile(t, conn, settingsNameOnceDown)

	var pipelineID string
	if err := conn.QueryRow(ctx, `INSERT INTO pipeline (name) VALUES ('Stage twins') RETURNING id::text`).Scan(&pipelineID); err != nil {
		t.Fatalf("seeding the pipeline the stages share: %v", err)
	}
	lists := settingsLists(pipelineID)
	seeded := map[string][]string{}
	// Reset keeps the lookup tables, so their twins would outlive this test.
	t.Cleanup(func() {
		for table, ids := range seeded {
			if _, err := conn.Exec(ctx, `DELETE FROM `+table+` WHERE id = ANY($1::uuid[])`, ids); err != nil {
				t.Errorf("removing the seeded %s rows: %v", table, err)
			}
		}
	})
	serial := 0
	seed := func(list settingsList, name, createdAt string) string {
		t.Helper()
		serial++
		var id string
		err := conn.QueryRow(ctx, list.insert+` RETURNING id::text`,
			pgx.NamedArgs{"name": name, "created_at": createdAt, "n": serial}).Scan(&id)
		if err != nil {
			t.Fatalf("%s: seeding %q: %v", list.table, name, err)
		}
		seeded[list.table] = append(seeded[list.table], id)
		return id
	}

	longName := "Partner referral " + strings.Repeat("x", 100)
	type expectation struct {
		list          settingsList
		id, want, why string
	}
	var expected []expectation
	held := map[string]string{}
	for _, list := range lists {
		keeper := seed(list, longName, "2026-01-01T00:00:00Z")
		spaced := seed(list, " "+strings.ToUpper(longName)+" ", "2026-02-01T00:00:00Z")
		first := seed(list, "Gone dark", "2026-01-01T00:00:00Z")
		colliding := seed(list, "gone dark", "2026-02-01T00:00:00Z")
		seed(list, "Gone dark (duplicate 1)", "2026-03-01T00:00:00Z")
		held[list.table] = keeper
		expected = append(expected,
			expectation{list, keeper, longName, "the oldest keeps its name"},
			expectation{list, spaced, strings.ToUpper(longName)[:106] + " (duplicate 1)", "a twin is renamed within the label limit"},
			expectation{list, first, "Gone dark", "the oldest keeps its name"},
			expectation{list, colliding, "gone dark (" + colliding + ")", "a rename that would collide carries the row id"})
	}
	var managed string
	if err := conn.QueryRow(ctx, `
		INSERT INTO knowledge_corpus (name, topic_statement, captured_by, managed_source, created_at)
		VALUES ('Margince handbook', 'Handbook', 'system', 'handbook', '2026-05-01T00:00:00Z') RETURNING id::text`).Scan(&managed); err != nil {
		t.Fatalf("seeding a managed document set: %v", err)
	}
	seeded["knowledge_corpus"] = append(seeded["knowledge_corpus"], managed)
	corpora := lists[len(lists)-1]
	userTwin := seed(corpora, " margince HANDBOOK", "2026-01-01T00:00:00Z")
	expected = append(expected, expectation{
		corpora, userTwin, "margince HANDBOOK (duplicate 1)",
		"a user's set named like the managed one is renamed, however old",
	})

	applyMigrationFile(t, conn, settingsNameOnceUp)

	for _, c := range expected {
		var name string
		if err := conn.QueryRow(ctx, `SELECT `+c.list.column+` FROM `+c.list.table+` WHERE id = $1`, c.id).Scan(&name); err != nil {
			t.Fatalf("reading %s %s: %v", c.list.table, c.id, err)
		}
		if name != c.want {
			t.Errorf("%s: %s: name %q, want %q", c.list.table, c.why, name, c.want)
		}
	}
	var managedName string
	if err := conn.QueryRow(ctx, `SELECT name FROM knowledge_corpus WHERE id = $1`, managed).Scan(&managedName); err != nil || managedName != "Margince handbook" {
		t.Errorf("the managed document set did not keep its name beside a user's twin (%q, %v)", managedName, err)
	}

	for _, list := range lists {
		expectRefusal(t, conn, list, "GONE DARK ", "a second spelling of a held name was admitted")
		if _, err := conn.Exec(ctx, list.release, held[list.table]); err != nil {
			t.Fatalf("%s: taking the keeper out of use: %v", list.table, err)
		}
		if !list.releasesName {
			expectRefusal(t, conn, list, strings.ToLower(longName), "a retired entry gave up the name it can be restored to")
			continue
		}
		seed(list, strings.ToLower(longName), "2026-04-01T00:00:00Z")
	}
}

// expectRefusal inserts name into list and wants the list's own index to refuse it.
func expectRefusal(t *testing.T, conn *pgx.Conn, list settingsList, name, why string) {
	t.Helper()
	_, err := conn.Exec(context.Background(), list.insert, pgx.NamedArgs{"name": name, "created_at": "2026-05-01T00:00:00Z", "n": 9999})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != list.index {
		t.Errorf("%s: %s: %v", list.table, why, err)
	}
}

func TestALeadNameIsHeldToALabelAndAnEarlierLongerOneStays(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	applyMigrationFile(t, conn, leadNameBoundDown)
	longName := strings.Repeat("n", 121)
	var earlier string
	if err := conn.QueryRow(ctx, `INSERT INTO lead (full_name, source, captured_by) VALUES ($1, 'manual', 'test') RETURNING id::text`,
		longName).Scan(&earlier); err != nil {
		t.Fatalf("seeding a lead named past the label limit: %v", err)
	}

	applyMigrationFile(t, conn, leadNameBoundUp)

	if _, err := conn.Exec(ctx, `UPDATE lead SET title = 'Buyer' WHERE id = $1`, earlier); err != nil {
		t.Errorf("an unchanged earlier name blocked an unrelated edit: %v", err)
	}
	_, err := conn.Exec(ctx, `INSERT INTO lead (full_name, source, captured_by) VALUES ($1, 'manual', 'test')`, longName)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "22001" || pgErr.ColumnName != "full_name" {
		t.Errorf("a new lead name past the label limit was admitted: %v", err)
	}
}
