// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An import is never swept on the day it arrives, and an unconverted lead is
// archived rather than anonymized by default.
//
// The defect: an importer states the source system's dates, so a lead first
// seen in 2023 arrived with a 2023 created_at, and the first retention pass
// after the import anonymized every lead older than a year — 1,200 of them on
// a rehearsal install within ninety minutes. Retention now also asks how long a
// record has been in THIS installation (entered_at, which nothing writes after
// the insert — backend/gates/enteredatwriters_test.go; an activity's
// created_at, which no import restates).

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func runRetentionPass(t *testing.T, e *Env) {
	t.Helper()
	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
}

// Records that arrive today carrying old dates — the way an import backdates
// them, with a direct UPDATE of created_at after the insert — are left alone by
// every policy.
func TestAnImportedRecordIsNotSweptOnArrival(t *testing.T) {
	e := Setup(t)
	SeedRetentionPolicies(t, e)
	lead, contact, activity := ids.NewV7(), ids.NewV7(), ids.NewV7()
	ctx := context.Background()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO lead (id, full_name, email, status, source, captured_by)
			VALUES ($1, 'Imported Lead', 'imported@old.example', 'new', 'import', 'human:x')`, lead); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, 'Imported Contact', 'import', 'human:x')`, contact); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, occurred_at, source, source_system, source_id, captured_by)
			VALUES ($1, 'note', 'Old note', 'text', now() - interval '4000 days', 'import', 'mirror:test', 'n-1', 'human:x')`,
			activity); err != nil {
			return err
		}
		// The importer's backdate: the source system's dates, written after the
		// fact.
		if _, err := tx.Exec(ctx, `
			UPDATE lead SET created_at = now() - interval '4000 days' WHERE id = $1`, lead); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE contact SET created_at = now() - interval '4000 days' WHERE id = $1`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	runRetentionPass(t, e)

	var leadName, contactName string
	var leadArchived, activityArchived bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT full_name, archived_at IS NOT NULL FROM lead WHERE id = $1`,
			lead).Scan(&leadName, &leadArchived); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT full_name FROM contact WHERE id = $1`, contact).Scan(&contactName); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM activity WHERE id = $1`, activity).Scan(&activityArchived)
	}); err != nil {
		t.Fatal(err)
	}
	if leadName != "Imported Lead" || leadArchived {
		t.Errorf("a lead imported today was acted on: name %q, archived %v", leadName, leadArchived)
	}
	if contactName != "Imported Contact" {
		t.Errorf("a contact imported today was acted on: name %q", contactName)
	}
	if activityArchived {
		t.Error("an activity imported today was archived for the age of the message it records")
	}
}

// A fresh installation's unconverted-lead policy archives: the lead leaves
// every list and keeps its name, so a rep who meets them again can restore it.
func TestTheDefaultPolicyArchivesAnUnconvertedLeadAndKeepsIt(t *testing.T) {
	e := Setup(t)
	lead := ids.NewV7()
	ctx := context.Background()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM retention_policy`); err != nil {
			return err
		}
		if err := consent.SeedDefaultRetentionTx(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO lead (id, full_name, email, status, source, captured_by, created_at, entered_at)
			VALUES ($1, 'Cold Lead', 'cold@old.example', 'new', 'manual', 'human:x',
			        now() - interval '400 days', now() - interval '400 days')`, lead)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	runRetentionPass(t, e)

	var name string
	var email *string
	var archived bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT full_name, email, archived_at IS NOT NULL FROM lead WHERE id = $1`,
			lead).Scan(&name, &email, &archived)
	}); err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("the over-age unconverted lead was not archived by the default policy")
	}
	if name != "Cold Lead" || email == nil || *email != "cold@old.example" {
		t.Fatalf("the archived lead lost its identity (name %q, email %v); the default archives, it does not anonymize", name, email)
	}
}

// An installation that switches its unconverted-lead policy from archive to
// anonymize reaches the leads the archive already took: an archived lead still
// holds the person's details, and a selector that skipped archived rows would
// keep them forever.
func TestAnAnonymizePolicyReachesALeadTheArchiveAlreadyTook(t *testing.T) {
	e := Setup(t)
	lead := ids.NewV7()
	ctx := context.Background()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM retention_policy`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('lead', 'unconverted', 365, 'anonymize')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO lead (id, full_name, email, status, source, captured_by, created_at, entered_at, archived_at)
			VALUES ($1, 'Archived Cold Lead', 'archived@old.example', 'new', 'manual', 'human:x',
			        now() - interval '400 days', now() - interval '400 days', now() - interval '30 days')`, lead)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	runRetentionPass(t, e)

	var name string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT full_name FROM lead WHERE id = $1`, lead).Scan(&name)
	}); err != nil {
		t.Fatal(err)
	}
	if name != "Anonymized Lead" {
		t.Fatalf("an archived over-age lead kept its name %q under an anonymize policy", name)
	}
}
