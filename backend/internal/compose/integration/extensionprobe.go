// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// ProbeExtensionTable creates an ext table for one test, as a unit migration
// would. It declares one activity link and one personal column, and files one
// row linked to the activity.
func ProbeExtensionTable(t *testing.T, owner *pgx.Conn, name string, activity ids.UUID) (extprivacy.Registry, ids.UUID) {
	t.Helper()
	table := pgx.Identifier{"ext", name}.Sanitize()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`CREATE TABLE ` + table + ` (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), activity_id uuid, body text NOT NULL)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ` + table + ` TO margince_app`,
	} {
		if _, err := owner.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("preparing the probe table (%s): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
			t.Errorf("removing the probe table: %v", err)
		}
	})
	row := ids.NewV7()
	if _, err := owner.Exec(t.Context(), `INSERT INTO `+table+` (id, activity_id, body) VALUES ($1, $2, 'probe')`, row, activity); err != nil {
		t.Fatalf("filing the probe row: %v", err)
	}
	return extprivacy.Declared([]privacy.Table{{
		Name: name, Links: []privacy.Link{{Column: "activity_id", Record: privacy.Activity}},
		Personal: []string{"body"}, Retention: jurisdiction.CommercialCorrespondence,
	}}), row
}
