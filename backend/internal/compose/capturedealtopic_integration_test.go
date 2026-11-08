// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// countingTopicModel answers like aboutEveryDeal and counts its calls.
type countingTopicModel struct{ calls int }

func (m *countingTopicModel) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	m.calls++
	return aboutEveryDeal.Complete(ctx, req)
}

// The check asks only about the whole candidate set it was given, and only
// for the input it read. A candidate that no longer reads, or an input that
// changed since the question was named, asks no model and abstains.
func TestTheTopicCheckAsksOnlyAboutTheWholeUnchangedQuestion(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	brain := &countingTopicModel{}
	check := NewDealTopicCheck(e.Pool, brain)
	seat := principal.WithCorrelationID(e.Admin(), ids.NewV7())
	mail := ids.From[ids.ActivityKind](f.mail)
	digest := func(deals ...ids.UUID) string {
		t.Helper()
		var named string
		if err := InstallationDB(e.Pool).Tx(seat, func(tx pgx.Tx) error {
			var err error
			named, err = check.Digest(seat, tx, mail, deals)
			return err
		}); err != nil {
			t.Fatalf("naming the question: %v", err)
		}
		return named
	}

	whole := digest(f.deal)
	if whole == "" {
		t.Fatal("a question about one readable deal has no digest, so this case proves nothing")
	}
	if partial := digest(f.deal, ids.NewV7()); partial != "" {
		t.Fatal("a question about a deal that no longer reads was named, so the model would judge a partial set")
	}
	if _, err := check.Check(seat, mail, []ids.UUID{f.deal}, whole+"-changed"); !errors.Is(err, capture.ErrTopicAbstained) {
		t.Fatalf("an answer for a changed input reports %v, want an abstention", err)
	}
	if brain.calls != 0 {
		t.Fatalf("the model was asked %d times for questions that could not stand, want none", brain.calls)
	}
	topic, err := check.Check(seat, mail, []ids.UUID{f.deal}, whole)
	if err != nil || brain.calls != 1 || topic.Deal != f.deal {
		t.Fatalf("the whole, unchanged question answered %+v, %v after %d calls, want the deal after one", topic, err, brain.calls)
	}
}
