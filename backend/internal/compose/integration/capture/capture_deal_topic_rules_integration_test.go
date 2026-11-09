// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// What the deal topic check may show the model: only what the capturing seat
// may read. And when its answer stands: only for the input it was given.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The model sees a candidate's project name only where the capturing seat
// may read projects.
func TestTheTopicCheckShowsAProjectNameOnlyUnderTheProjectGrant(t *testing.T) {
	for _, c := range []struct {
		name     string
		revoked  bool
		wantName bool
	}{
		{"with the project grant", false, true},
		{"without it", true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e := env.e
			w := newDealWorld(t, e)
			w.employee(t, "alice@acme.example", w.customer)
			rollout := w.project(t, "Warehouse rollout programme")
			w.dealOnProject(t, "Acme licences", rollout.ID)
			if c.revoked {
				revokeCaptureGrant(t, e, "project")
			}
			env.sync(t, mailFrom("alice@acme.example", "pn1@acme.example", "", "status"))
			sent := strings.Join(env.topic.sent(), "\n")
			if sent == "" {
				t.Fatal("the topic check asked nothing, so this case proves nothing")
			}
			if got := strings.Contains(sent, "Warehouse rollout programme"); got != c.wantName {
				t.Fatalf("the prompt names the project: %v, want %v", got, c.wantName)
			}
		})
	}
}

// The memo names the whole input, so a renamed deal asks again.
func TestAChangedDealAsksTheTopicCheckAgain(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")
	env.topic.answer = func(string) (string, float64) { return "not_about", 0.95 }
	raw := mailFrom("alice@acme.example", "ren1@acme.example", "", "status")
	env.sync(t, raw)
	env.sync(t, raw)
	if asked := env.topic.asked(); asked != 1 {
		t.Fatalf("the model was asked %d times for an unchanged question, want once", asked)
	}
	renamed := "Acme rollout phase two"
	if _, err := w.store.UpdateDeal(w.ctx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{Name: &renamed}); err != nil {
		t.Fatalf("renaming the deal: %v", err)
	}
	env.sync(t, raw)
	if asked := env.topic.asked(); asked != 2 {
		t.Fatalf("the model was asked %d times after the deal was renamed, want again", asked)
	}
}

// An answer is for the input the model read. When the deal changes while the
// model answers, the second transaction files nothing.
func TestAnAnswerForAnInputThatChangedFilesNothing(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")
	env.topic.during = func() {
		renamed := "Acme support renewal"
		if _, err := w.store.UpdateDeal(w.ctx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{Name: &renamed}); err != nil {
			t.Errorf("renaming the deal while the model answers: %v", err)
		}
	}
	env.sync(t, mailFrom("alice@acme.example", "chg1@acme.example", "", "status"))
	wantDeals(t, e, "chg1@acme.example")
}
