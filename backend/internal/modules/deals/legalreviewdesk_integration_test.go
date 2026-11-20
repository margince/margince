// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// Who is told that a legal review waits, and when the word is taken back.

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordingDesk stands in for compose's desk, a boundary this package may not
// import; compose's own suite runs the real one.
type recordingDesk struct {
	reviewers []ids.UUID
	raised    []ReviewRequest
	settled   []ids.UUID
}

func (d *recordingDesk) Reviewers(context.Context, pgx.Tx) ([]ids.UUID, error) {
	return d.reviewers, nil
}

func (d *recordingDesk) Raise(_ context.Context, _ pgx.Tx, r ReviewRequest) error {
	d.raised = append(d.raised, r)
	return nil
}

func (d *recordingDesk) Settle(_ context.Context, _ pgx.Tx, id ids.UUID) error {
	d.settled = append(d.settled, id)
	return nil
}

func TestOnlySeatsOnTheOtherSideOfTheDealAreToldAReviewWaits(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	owner := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Olga Owner')`,
		owner, "owner-"+owner.String()+"@config.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE deal SET owner_id = $1 WHERE id = $2`, owner, dealID); err != nil {
		t.Fatal(err)
	}
	e.desk.reviewers = []ids.UUID{e.reviewer, e.admin, owner}

	review := e.request(t, dealID, nil)

	if len(e.desk.raised) != 1 {
		t.Fatalf("the desk was asked %d times, want once", len(e.desk.raised))
	}
	got := e.desk.raised[0]
	if got.ReviewID != review || got.DealID != dealID.UUID || !slices.Equal(got.Seats, []ids.UUID{e.reviewer}) {
		t.Errorf("announced %+v, want review %s to the reviewer alone, not the requester or the deal's owner", got, review)
	}
	if _, err := e.store.RequestLegalReview(e.asRequester(), dealID, RequestLegalReviewInput{}); err == nil {
		t.Fatal("a second request on the deal was admitted")
	}
	if len(e.desk.raised) != 1 {
		t.Errorf("the refused second request told the desk again: %d announcements", len(e.desk.raised))
	}
}

func TestAnsweringOrWithdrawingAReviewSettlesTheWordToReviewers(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	e.desk.reviewers = []ids.UUID{e.reviewer}

	answered := e.request(t, dealID, nil)
	e.answer(t, answered, ReviewChangesNeeded)
	again := e.request(t, dealID, &answered)
	if _, err := e.store.WithdrawLegalReview(e.asRequester(), again); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(e.desk.settled, []ids.UUID{answered, again}) {
		t.Errorf("settled %v, want the answered review then the withdrawn one", e.desk.settled)
	}
	if len(e.desk.raised) != 2 {
		t.Errorf("announcements = %d, want one per request", len(e.desk.raised))
	}
}
