// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package evidencereread

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type pageJob struct{}

func (pageJob) Kind() string        { return "stage_evidence_reread" }
func (pageJob) SentFrom() string    { return "buyer-corp.test" }
func (pageJob) PageAfter() ids.UUID { return ids.UUID{} }
func bound(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func work(w *Worker[pageJob]) error {
	w.Log = slog.New(slog.DiscardHandler)
	return w.Work(context.Background(), &river.Job[pageJob]{Args: pageJob{}})
}

func TestARoleWithNoModelLaneCompletesWithoutAnInserter(t *testing.T) {
	asked := false
	err := work(&Worker[pageJob]{Bind: bound, Queue: func() (Enqueuer, error) {
		asked = true
		return nil, errors.New("no inserter")
	}})
	if err != nil || asked {
		t.Errorf("a role that reads nothing answered %v and asked for an inserter: %t", err, asked)
	}
}

func TestAPageWhoseInserterCannotBeBuiltFails(t *testing.T) {
	missing := errors.New("no inserter")
	err := work(&Worker[pageJob]{Bind: bound, Reads: true, Queue: func() (Enqueuer, error) { return nil, missing }})
	if !errors.Is(err, missing) {
		t.Errorf("the page answered %v, want the inserter's fault so River retries it", err)
	}
}

func TestAFailedBindingIsRefusedBeforeAnythingIsRead(t *testing.T) {
	refused := errors.New("no workspace")
	err := work(&Worker[pageJob]{
		Reads: true,
		Bind:  func(context.Context) (context.Context, error) { return nil, refused },
	})
	if !errors.Is(err, refused) {
		t.Errorf("the page answered %v, want the binding's refusal", err)
	}
}
