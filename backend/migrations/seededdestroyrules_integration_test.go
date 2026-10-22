// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// An installation seeded with the transcript-erase and contact-anonymize rules
// gets both switched off, unless an admin changed them, and keeps its
// raw_capture window. The shipped migration is replayed over each shape.

import (
	"context"
	"testing"
)

const seededDestroyRulesMigration = "1792477889_seeded_rules_that_destroy_an_old_record_are_switched_off.up.sql"

type policyState struct {
	days    int
	action  string
	enabled bool
}

func TestTheSeededRulesThatDestroyAnOldRecordAreSwitchedOff(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	reset := func() {
		if _, err := conn.Exec(ctx, `DELETE FROM retention_policy
			 WHERE (object_type, coalesce(category, '')) IN
			       (('activity', 'transcript'), ('contact', 'no_consent_no_deal'), ('raw_capture', ''))`); err != nil {
			t.Fatalf("clearing the policies: %v", err)
		}
	}
	t.Cleanup(reset)
	state := func(t *testing.T, objectType, category string) policyState {
		t.Helper()
		var s policyState
		if err := conn.QueryRow(ctx, `SELECT retain_days, action, enabled FROM retention_policy
			 WHERE object_type = $1 AND coalesce(category, '') = $2`, objectType, category).
			Scan(&s.days, &s.action, &s.enabled); err != nil {
			t.Fatalf("reading the %s/%s policy: %v", objectType, category, err)
		}
		return s
	}
	plant := func(t *testing.T, sql string) {
		t.Helper()
		reset()
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("planting the policies: %v", err)
		}
	}

	t.Run("the rows still as seeded are switched off and raw_capture keeps its window", func(t *testing.T) {
		plant(t, `INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis) VALUES
			('activity', 'transcript', 365, 'erase', 'storage_limitation'),
			('contact', 'no_consent_no_deal', 730, 'anonymize', 'storage_limitation'),
			('raw_capture', NULL, 730, 'erase', 'storage_limitation')`)
		replayMigration(t, conn, seededDestroyRulesMigration)
		if got := state(t, "activity", "transcript"); got != (policyState{365, "erase", false}) {
			t.Errorf("the seeded transcript rule after the migration: %+v, want it kept at 365 days, erase, and disabled", got)
		}
		if got := state(t, "contact", "no_consent_no_deal"); got != (policyState{730, "anonymize", false}) {
			t.Errorf("the seeded contact rule after the migration: %+v, want it kept at 730 days, anonymize, and disabled", got)
		}
		if got := state(t, "raw_capture", ""); got != (policyState{730, "erase", true}) {
			t.Errorf("raw_capture after the migration: %+v, want it untouched at 730 days, erase, enabled", got)
		}
	})

	t.Run("a rule whose window an admin changed stays as they set it", func(t *testing.T) {
		plant(t, `INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis) VALUES
			('activity', 'transcript', 400, 'erase', 'storage_limitation'),
			('contact', 'no_consent_no_deal', 1000, 'anonymize', 'storage_limitation')`)
		replayMigration(t, conn, seededDestroyRulesMigration)
		if got := state(t, "activity", "transcript"); got != (policyState{400, "erase", true}) {
			t.Errorf("the transcript rule with an edited window after the migration: %+v, want it still enabled", got)
		}
		if got := state(t, "contact", "no_consent_no_deal"); got != (policyState{1000, "anonymize", true}) {
			t.Errorf("the contact rule with an edited window after the migration: %+v, want it still enabled", got)
		}
	})

	t.Run("a rule whose basis an admin changed stays as they set it", func(t *testing.T) {
		plant(t, `INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis) VALUES
			('activity', 'transcript', 365, 'erase', 'legal_obligation'),
			('contact', 'no_consent_no_deal', 730, 'anonymize', 'legal_obligation')`)
		replayMigration(t, conn, seededDestroyRulesMigration)
		if got := state(t, "activity", "transcript"); got != (policyState{365, "erase", true}) {
			t.Errorf("the transcript rule with an edited basis after the migration: %+v, want it still enabled", got)
		}
		if got := state(t, "contact", "no_consent_no_deal"); got != (policyState{730, "anonymize", true}) {
			t.Errorf("the contact rule with an edited basis after the migration: %+v, want it still enabled", got)
		}
	})
}
