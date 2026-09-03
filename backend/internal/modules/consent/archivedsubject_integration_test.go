// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What may still be recorded once the subject is archived — including
// anonymized under Art. 17, which stamps archived_at and leaves the row
// standing.
//
// The split is by STATE, not by subject. A withdrawal must stay recordable
// because suppression depends on it, and it is the case you most want working
// after somebody has asked to be forgotten. A grant must not: erasure
// anonymizes in place, so the person row survives and an erased subject would
// go on accruing fresh person_consent, consent_event, audit and outbox rows —
// the accrual privacy/erasure.go's deletePreferenceToken destroys the emailed
// capability to stop.
//
// Both subject kinds run because Record takes either and dispatches on which,
// writing a different column of person_consent for each.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asBoundedRep is the seat the withdrawal arm needs to mean anything. Both
// consent harnesses bind RowScopeAll, and auth.EnsureWritable degenerates to a
// no-op for an unbounded actor — EnsureVisible returns nil on an empty scope
// clause and ensureWriteAuthority returns nil outright — so a suite that only
// ran unbounded would stay green with the withdrawal's probe deleted entirely.
// Since the whole change IS a choice of probe, one arm has to be bounded.
func boundedRepCtx(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"person": {Read: true, Update: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

func TestAnArchivedSubjectTakesAWithdrawalButNotAGrant(t *testing.T) {
	// A withdrawal is admitted against an archived subject, NOT against one the
	// caller cannot reach. The two are different questions and the permissive
	// probe still answers the second: without this arm the withdrawal branch
	// could lose its probe altogether and every other assertion here would
	// still pass.
	t.Run("a withdrawal is still row-gated", func(t *testing.T) {
		e := setupChannelConsent(t)
		archiveConsentSubject(t, e.owner, "person", e.person.UUID)
		bounded := boundedRepCtx(e.ws, ids.NewV7())
		// ErrPermissionDenied rather than ErrNotFound, and the difference says
		// which half refused: a person row defaults to workspace visibility, so
		// this rep can SEE the subject — the scope clause admits them — and it
		// is the write-authority arm that stops them, the row being owned by
		// nobody they are. Existence-hiding has nothing left to hide once the
		// caller has been shown the row.
		if _, err := e.store.Record(bounded, RecordInput{
			PersonID: e.person, PurposeID: e.newsletter, NewState: string(StateWithdrawn),
		}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("withdrawing for a subject the caller may read but not change: got %v, want permission denied", err)
		}
		var rows int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM person_consent WHERE person_id = $1`, e.person).Scan(&rows); err != nil {
			t.Fatalf("count person_consent: %v", err)
		}
		if rows != 0 {
			t.Errorf("person_consent rows = %d, want 0 — a refused withdrawal still wrote", rows)
		}
	})

	t.Run("a person", func(t *testing.T) {
		e := setupChannelConsent(t)
		archiveConsentSubject(t, e.owner, "person", e.person.UUID)
		assertConsentSplit(e.ctx, t, e.store, e.owner, RecordInput{
			PersonID: e.person, PurposeID: e.newsletter,
		}, "person_id", e.person.UUID)

		// The slower route to the same grant used to be double-opt-in issuance,
		// which reached person_consent without going through Record. There is
		// no such route now — issuance refuses at the handler and mints
		// nothing — so the only thing left to hold is that the table stayed
		// empty for this subject.
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
//
// column, like archiveConsentSubject's table, is a test-local literal naming
// person_consent's person or lead arm — never caller input.
// ctx leads rather than t, against this package's usual helper shape: revive's
// context-as-argument rule is enforced by the lint gate and rejects the other
// order outright.
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
