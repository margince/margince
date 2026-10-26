// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"bytes"
	"context"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestExtensionSettingsSeedOnceAndKeepAnEdit(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, migratedDatabase(t, "_extsettings"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("closing: %v", err)
		}
	})
	exts := []extension.Extension{{
		Name:     "finance-demo",
		Settings: fstest.MapFS{extension.SettingsFile: {Data: []byte(`{"vat_rate": 0.19, "region": "de"}`)}},
	}}

	var first bytes.Buffer
	if err := seedExtensionSettings(ctx, conn, exts, &first); err != nil {
		t.Fatal(err)
	}
	if got := first.String(); got != "finance-demo: stored 2, kept 0\n" {
		t.Fatalf("first run reported %q", got)
	}

	if _, err := conn.Exec(ctx, `UPDATE setting SET value = '0.07' WHERE key = 'ext_finance_demo.vat_rate'`); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := seedExtensionSettings(ctx, conn, exts, &second); err != nil {
		t.Fatal(err)
	}
	if got := second.String(); got != "finance-demo: stored 0, kept 2\n" {
		t.Fatalf("second run reported %q", got)
	}
	var rate string
	if err := conn.QueryRow(ctx, `SELECT value::text FROM setting WHERE key = 'ext_finance_demo.vat_rate'`).Scan(&rate); err != nil {
		t.Fatal(err)
	}
	if rate != "0.07" {
		t.Fatalf("the seed overwrote an edited value: %s", rate)
	}
}

func TestExtensionSettingsABadFileStoresNothing(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, migratedDatabase(t, "_extsettingsbad"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("closing: %v", err)
		}
	})
	exts := []extension.Extension{
		{Name: "aa", Settings: fstest.MapFS{extension.SettingsFile: {Data: []byte(`{"ok": 1}`)}}},
		{Name: "bb", Settings: fstest.MapFS{extension.SettingsFile: {Data: []byte(`{"Bad-Key": 1}`)}}},
	}

	if err := seedExtensionSettings(ctx, conn, exts, &bytes.Buffer{}); err == nil {
		t.Fatal("a bad declaration was applied")
	}
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM setting WHERE key LIKE 'ext\_%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d rows were stored from a run that failed", n)
	}
}
