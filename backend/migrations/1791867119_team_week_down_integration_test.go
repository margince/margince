// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Rolling back the team week's patterns restores the focus vocabulary as it
// was before: narrow, and validated.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestTheTeamWeekDownRestoresAValidatedFocusVocabulary(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1791867120")); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1791867120"), 2); err != nil {
		t.Fatalf("down: %v", err)
	}
	var validated bool
	var definition string
	if err := conn.QueryRow(ctx, `
		SELECT convalidated, pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conname = 'team_weekly_review_rep_focus_kind_check'`).Scan(&validated, &definition); err != nil {
		t.Fatal(err)
	}
	if !validated {
		t.Error("the rolled-back focus vocabulary is NOT VALID, which the schema before it was not")
	}
	if strings.Contains(definition, "stage_regressions") {
		t.Errorf("the rolled-back vocabulary still admits the new kinds: %s", definition)
	}
}
