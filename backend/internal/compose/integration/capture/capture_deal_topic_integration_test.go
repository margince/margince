// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The deal topic check end to end. Capture files a mail on a deal it only
// suggests when the model is confident the mail is about it. It never asks the
// model about a mail its project or proven thread already files.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// One open deal on a project, and a mail the model judges by the verdict
// given. Only a confident "about" files the mail, and the project follows the
// deal only then.
func TestASuggestedDealIsFiledOnlyOnAConfidentAnswer(t *testing.T) {
	for _, c := range []struct {
		name       string
		verdict    string
		confidence float64
		filed      bool
	}{
		{"not about it", "not_about", 0.95, false},
		{"about it", "about", 0.9, true},
		{"about it, unsure", "about", 0.7, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e := env.e
			w := newDealWorld(t, e)
			w.employee(t, "alice@acme.example", w.customer)
			rollout := w.project(t, "Rollout")
			deal := w.dealOnProject(t, "Acme rollout", rollout.ID)
			env.topic.answer = func(string) (string, float64) { return c.verdict, c.confidence }
			raw := mailFrom("alice@acme.example", "topic1@acme.example", "", "Invoice for the maintenance hours")
			env.sync(t, raw)
			env.sync(t, raw)

			if c.filed {
				wantDeals(t, e, "topic1@acme.example", deal)
				if got := linkedProject(t, e, "topic1@acme.example"); got != rollout.ID {
					t.Fatalf("the filed mail is on project %s, want the deal's %s", got, rollout.ID)
				}
				return
			}
			wantDeals(t, e, "topic1@acme.example")
			if got := linkedProject(t, e, "topic1@acme.example"); !got.IsZero() {
				t.Fatalf("a mail on no deal took the deal's project %s", got)
			}
			if asked := env.topic.asked(); asked != 1 {
				t.Fatalf("the model was asked %d times over two captures, want once: the memo holds the answer", asked)
			}
		})
	}
}

// With no model, capture files nothing on a suggestion. The refile pass files
// the mail once a model answers.
func TestASuggestedDealWaitsForAModel(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")
	env.topic.unconfigured = true
	env.sync(t, mailFrom("alice@acme.example", "wait1@acme.example", "", "status"))
	wantDeals(t, e, "wait1@acme.example")

	env.topic.unconfigured = false
	refile(t, refiler(env, w), ids.From[ids.ActivityKind](activityOf(t, e, "wait1@acme.example")))
	wantDeals(t, e, "wait1@acme.example", deal)
}

// A stakeholder's deal and a meeting are suggestions too: the model is asked
// before either is filed.
func TestAStakeholderDealAndAMeetingAreChecked(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	bob := w.employee(t, "bob@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	w.stakeholder(t, bob, reporting)
	env.topic.answer = func(string) (string, float64) { return "not_about", 0.95 }
	env.sync(t, mailFrom("bob@acme.example", "staked1@acme.example", "", "Office party"))
	env.syncAsKind(t, map[string]string{"meet1@acme.example": "meeting"},
		mailFrom("bob@acme.example", "meet1@acme.example", "", "Lunch"))
	wantDeals(t, e, "staked1@acme.example")
	wantDeals(t, e, "meet1@acme.example")
	if asked := env.topic.asked(); asked != 2 {
		t.Fatalf("the model was asked %d times, want once for the mail and once for the meeting", asked)
	}
}

// The project and a proven thread are explicit evidence: the model is never
// asked about a mail they file.
func TestExplicitEvidenceFilesWithoutTheModel(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	crm := w.project(t, "CRM project")
	crmDeal := w.dealOnProject(t, "CRM licences", crm.ID)
	operations := w.deal(t, "Acme operations")
	env.syncSent(t, map[string]bool{"thr1@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "thr1@myco.example", ""))
	relinkAsColleague(t, e, w, activityOf(t, e, "thr1@myco.example"), "deal", operations)

	env.topic.forbid = t
	env.sync(t, mailFrom("alice@acme.example", "pj1@acme.example", "", "["+crm.Key+"] kickoff"))
	wantDeals(t, e, "pj1@acme.example", crmDeal)
	env.sync(t, authenticated(mailFrom("alice@acme.example", "thr2@acme.example", "thr1@myco.example", "Re: project"), "acme.example"))
	wantDeals(t, e, "thr2@acme.example", operations)
}

// Several open deals: a confident pick files the mail and asks nobody, a
// confident "none" files nothing and asks nobody, and doubt asks the colleague.
func TestAnAmbiguousMailIsCheckedBeforeAnyoneIsAsked(t *testing.T) {
	for _, c := range []struct {
		name  string
		pick  func(deal string) (string, float64)
		filed bool
		asked int
	}{
		{"one deal picked", nil, true, 0},
		{"none of them", func(string) (string, float64) { return "not_about", 0.95 }, false, 0},
		{"unsure", func(string) (string, float64) { return "unsure", 0.5 }, false, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e := env.e
			w := newDealWorld(t, e)
			w.employee(t, "alice@acme.example", w.customer)
			operations := w.deal(t, "Acme operations")
			w.deal(t, "Acme reporting")
			env.topic.answer = c.pick
			if c.pick == nil {
				env.topic.answer = pickDeal(operations)
			}
			env.syncSent(t, map[string]bool{"amb1@myco.example": true},
				email(captureOwner, "", "alice@acme.example", "amb1@myco.example", ""))
			if c.filed {
				wantDeals(t, e, "amb1@myco.example", operations)
			} else {
				wantDeals(t, e, "amb1@myco.example")
			}
			if n, _ := whichDealQuestions(t, e); n != c.asked {
				t.Fatalf("%d deal questions, want %d", n, c.asked)
			}
		})
	}
}

// pickDeal answers "about" for chosen and "not_about" for every other deal.
func pickDeal(chosen ids.UUID) func(string) (string, float64) {
	return func(deal string) (string, float64) {
		if deal == chosen.String() {
			return "about", 0.95
		}
		return "not_about", 0.95
	}
}
