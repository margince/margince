// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What may still be recorded once the subject is archived — including
// anonymized under Art. 17, which stamps archived_at and leaves the row
// standing.
//
// The split is by STATE, not by subject: a withdrawal must stay recordable
// because suppression depends on it and it is the case you most want working
// after somebody has asked to be forgotten, while a grant is a lawful-basis
// claim about a person the installation was told to forget. person_consent is
// a declared PII table, so a grant against an erased subject puts back a row
// the erasure had just deleted.
//
// Both subject kinds are exercised because Record takes either, and the two
// reach different scope clauses on the way to the same rule.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestAnArchivedSubjectTakesAWithdrawalButNotAGrant(t *testing.T) {
	t.Run("a person", func(t *testing.T) {
		e := setupChannelConsent(t)
		archiveConsentSubject(t, e.owner, "person", e.person.UUID)
		assertConsentSplit(e.ctx, t, e.store, e.owner, RecordInput{
			PersonID: e.person, PurposeID: e.newsletter,
		}, "person_id", e.person.UUID)

		// The slower route to the same grant. A double opt-in exists so the
		// subject can CONFIRM a grant, so issuing its token for an archived
		// person is the same lawful-basis claim arriving by post — and it is a
		// separate entry point that reaches person_consent without going
		// through Record at all.
		if _, err := e.store.IssueDoubleOptIn(e.ctx, e.person, e.doiNews, false); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("issuing a double opt-in for an archived subject: got %v, want not found", err)
		}
		var tokens int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM consent_doi_token WHERE person_id = $1`, e.person).Scan(&tokens); err != nil {
			t.Fatalf("count consent_doi_token: %v", err)
		}
		if tokens != 0 {
			t.Errorf("consent_doi_token rows = %d, want 0", tokens)
		}
	})

	t.Run("a lead", func(t *testing.T) {
		e := setupLeadConsent(t)
		archiveConsentSubject(t, e.owner, "lead", e.lead.UUID)
		assertConsentSplit(e.ctx, t, e.store, e.owner, RecordInput{
			LeadID: e.lead, PurposeID: e.newsletter,
		}, "lead_id", e.lead.UUID)
	})
}

// archiveConsentSubject retires the subject the way erasure leaves it: the row
// still there, archived_at stamped. Written on the owner connection because
// these suites hold no archive entry point, and the state under test is the
// archived row rather than how it came to be one.
//
// table is a test-local literal, never caller input.
func archiveConsentSubject(t *testing.T, owner *pgx.Conn, table string, id ids.UUID) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`UPDATE `+table+` SET archived_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("archive %s: %v", table, err)
	}
}

// assertConsentSplit records a grant and then a withdrawal against the same
// archived subject and purpose, and checks that only the second lands.
func assertConsentSplit(
	ctx context.Context, t *testing.T, store *Store, owner *pgx.Conn,
	in RecordInput, column string, id ids.UUID,
) {
	t.Helper()

	grant := in
	grant.NewState = string(StateGranted)
	if _, err := store.Record(ctx, grant); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("granting consent for an archived subject: got %v, want not found", err)
	}

	withdrawal := in
	withdrawal.NewState = string(StateWithdrawn)
	got, err := store.Record(ctx, withdrawal)
	if err != nil {
		t.Fatalf("withdrawing consent for an archived subject must stay possible, got %v", err)
	}
	if got.State != string(StateWithdrawn) {
		t.Errorf("recorded state = %q, want withdrawn", got.State)
	}

	// On disk, and exactly one row: the refused grant left nothing behind, so
	// the withdrawal is not sitting beside a lawful-basis claim it never
	// displaced.
	var rows int
	var state string
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*), coalesce(max(state), '') FROM person_consent WHERE `+column+` = $1`,
		id).Scan(&rows, &state); err != nil {
		t.Fatalf("read person_consent: %v", err)
	}
	if rows != 1 || state != string(StateWithdrawn) {
		t.Errorf("person_consent rows = %d state = %q, want 1 withdrawn", rows, state)
	}
}
