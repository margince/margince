// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheRecheckClosesAPhoneCollisionAfterItsNumberIsRemoved(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.seedContact(ctx, t, "Ola Berg", []string{"ola@phone.example"}, []string{"+49305550101"})
	right := e.seedContact(ctx, t, "Kim Lund", []string{"kim@phone.example"}, []string{"+49305550101"})
	pairs := openCandidatePairs(ctx, t, e, entityContact, right.UUID)
	if len(pairs) != 1 || pairs[0].OtherID != left.String() {
		t.Fatalf("shared number filed %+v, want one pair", pairs)
	}
	if got := e.recheck(t); got.Closed != 0 {
		t.Fatalf("a live shared number lost its pair: %+v", got)
	}
	if _, err := e.store.UpdateContact(ctx, right, UpdateContactInput{Phones: []ContactPhoneInput{}}); err != nil {
		t.Fatal(err)
	}
	if got := e.recheck(t); got.Closed != 1 {
		t.Fatalf("removed shared number: %+v, want one pair closed", got)
	}
	if queue, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityContact}); err != nil || len(queue) != 0 {
		t.Fatalf("the stale phone pair remains: %+v, error %v", queue, err)
	}
	if _, err := e.store.UpdateContact(ctx, right, UpdateContactInput{Phones: []ContactPhoneInput{
		{Phone: "+49305550101", PhoneType: "mobile", IsPrimary: true},
	}}); err != nil {
		t.Fatal(err)
	}
	if queue, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityContact}); err != nil || len(queue) != 1 {
		t.Fatalf("restored number did not reopen its pair: %+v, error %v", queue, err)
	}
	if n := e.countInWorkspace(ctx, t, `SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'contact'`); n != 1 {
		t.Fatalf("reopening made %d rows, want the original row", n)
	}
}

func TestAPhoneEditReopensAPairAConcurrentRecheckCloses(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedContact(ctx, t, "Ola Berg", []string{"ola@phone.example"}, []string{"+49305550101"})
	right := e.seedContact(ctx, t, "Kim Lund", []string{"kim@phone.example"}, []string{"+49305550101"})
	if _, err := e.store.UpdateContact(ctx, right, UpdateContactInput{Phones: []ContactPhoneInput{}}); err != nil {
		t.Fatal(err)
	}
	var edit <-chan error
	if err := e.store.tx(e.asRecheck(), func(tx pgx.Tx) error {
		pairs, err := lockOpenPairs(ctx, tx, ids.Nil)
		if err != nil {
			return err
		}
		if len(pairs) != 1 {
			t.Fatalf("recheck locked %d pairs, want one", len(pairs))
		}
		var pid int
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		edit = inBackground(func() error {
			_, err := e.store.UpdateContact(e.as(), right, UpdateContactInput{Phones: []ContactPhoneInput{
				{Phone: "+49305550101", PhoneType: "mobile", IsPrimary: true},
			}})
			return err
		})
		if finished, err := e.waitForWaitersOrDone(t, &heldCreate{pid: pid}, 1, edit); finished {
			t.Fatalf("the phone edit did not wait for the recheck: %v", err)
		}
		_, err = archiveDedupeCandidateTx(e.asRecheck(), tx, pairs[0].id, archiveReasonNoLongerFiled, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-edit; err != nil {
		t.Fatal(err)
	}
	queue, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityContact})
	if err != nil || len(queue) != 1 {
		t.Fatalf("the concurrent edit lost its pair: %+v, error %v", queue, err)
	}
}

func TestTheRecheckPreservesUnfamiliarAndRoutingEvidence(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, evidence := range []string{
		`null`, `{}`, `[]`, `[null]`,
		`[{"field":"matched_lane","signal":"exact_conflict"}]`,
	} {
		left := e.plainContact(ctx, t, "Ola Berg", ids.NewV7().String()+"@phone.example")
		right := e.plainContact(ctx, t, "Kim Lund", ids.NewV7().String()+"@phone.example")
		pair := e.fileOldRulePair(ctx, t, entityContact, left, right, identityConflictConfidence)
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE dedupe_candidate SET evidence = @evidence::jsonb WHERE id = @id`,
				pgx.StrictNamedArgs{"evidence": evidence, "id": pair})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.recheck(t); got.Closed != 0 {
		t.Fatalf("unfamiliar or routing evidence was closed: %+v", got)
	}
}

func TestTheRecheckKeepsNameEvidenceAfterASharedPhoneIsRemoved(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.seedContact(ctx, t, "Mia Schulz", []string{"mia@one.example"}, []string{"+49305550101"})
	right := e.seedContact(ctx, t, "Mia Schulz", []string{"mia@two.example"}, []string{"+49305550101"})
	// Force the phone filing path: a name pair can win the create's first write.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM dedupe_candidate`); err != nil {
			return err
		}
		_, err := (ContactResolution{ContactID: left, MatchedLane: lanePhone}).recordSharedPhone(ctx, tx, right, "manual", "test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateContact(ctx, right, UpdateContactInput{Phones: []ContactPhoneInput{}}); err != nil {
		t.Fatal(err)
	}
	if got := e.recheck(t); got.Closed != 0 {
		t.Fatalf("same-name pair closed with its phone: %+v", got)
	}
	assertCurrentPairEvidence(t, e, fieldFullName)
}

func TestTheRecheckShowsTheAddressVariantAfterASharedPhoneIsRemoved(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.seedContact(ctx, t, "Mia Schulz", []string{"mia@one.example"}, []string{"+49305550101"})
	right := e.seedContact(ctx, t, "Evan Karl", []string{"mia+work@one.example"}, []string{"+49305550101"})
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM dedupe_candidate`); err != nil {
			return err
		}
		_, err := (ContactResolution{ContactID: left, MatchedLane: lanePhone}).recordSharedPhone(ctx, tx, right, "manual", "test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateContact(ctx, right, UpdateContactInput{Phones: []ContactPhoneInput{}}); err != nil {
		t.Fatal(err)
	}
	if got := e.recheck(t); got.Closed != 0 {
		t.Fatalf("address variant closed with its phone: %+v", got)
	}
	assertCurrentPairEvidence(t, e, fieldEmail)
}

func assertCurrentPairEvidence(t *testing.T, e *dedupeEnv, wantField string) {
	t.Helper()
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		var score float64
		var field string
		if err := tx.QueryRow(e.as(), `SELECT confidence::float8, evidence->0->>'field' FROM dedupe_candidate`).Scan(&score, &field); err != nil {
			return err
		}
		if score >= 1 || field != wantField || (field == fieldEmail && score != 0) {
			t.Errorf("pair retained its phone explanation: score %v, field %q, want %q", score, field, wantField)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
