// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// A lane that was wired and did not answer must not have its fallback cached.
//
// The card is keyed by a fingerprint, and a stored card whose fingerprint still
// matches is served without asking the model again. So a floor card written
// during a timeout and saved under the current key stands until some unrelated
// fact moves — one transient outage, made permanent, with the reader seeing a
// deterministic card and nothing saying why.
//
// A lane that is ABSENT is the opposite case and must still be cached: it will
// answer no differently next time, so its floor is the real answer.
//
// This became reachable in a new way when the language entered the fingerprint.
// An installation switching to German mints a fresh key for every deal, and a
// lane that happens to be down while it does would freeze the ENGLISH floor as
// that installation's German card.

import (
	"context"
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// refusingLane stands for every way a wired lane fails to answer: a timeout, a
// spent budget, a reply the parser refuses.
type refusingLane struct{ asked bool }

func (l *refusingLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.asked = true
	return model.Response{}, errors.New("the lane did not answer")
}

// cardClock is a fixed clock. write() reads it only to stamp the card, and a
// real one would make two runs of the same test stamp different cards.
func cardClock() time.Time { return time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC) }

// cardFacts is the minimum a card can be written from AND asked about.
//
// The timeline row is load-bearing: write() does not ask a lane about a deal
// with nothing to cite, because every sentence the model could return would be
// dropped for want of a citation. A fixture with no activity therefore tests
// the refusal-to-ask, not the lane, and these tests are about the lane.
func cardFacts(t *testing.T) facts {
	t.Helper()
	return facts{
		deal: crmcontracts.Deal{Name: "Nordwind", Status: "open"},
		timeline: []crmcontracts.Activity{{
			Id:         openapi_types.UUID(ids.NewV7()),
			Kind:       "email",
			Subject:    ptr("Angebot"),
			OccurredAt: cardClock().Add(-48 * time.Hour),
		}},
		now:  cardClock(),
		lang: "de",
	}
}

func ptr[T any](v T) *T { return &v }

func TestAWiredLaneThatDidNotAnswerIsReportedSoItsFallbackIsNotCached(t *testing.T) {
	lane := &refusingLane{}
	s := &Service{lane: lane, now: cardClock}
	f := cardFacts(t)

	card, laneFailed := s.write(context.Background(), f, decideMove(f), project(f, decideMove(f)), true)

	if !lane.asked {
		t.Fatal("the lane was never asked, so this proves nothing about a lane that fails")
	}
	if !laneFailed {
		t.Error("a lane that was wired and refused reported success, so its English floor would be cached " +
			"under the current fingerprint and served until an unrelated fact moved")
	}
	// The reader still gets a working card — the degrade is declared. Refusing
	// the page would be the worse answer.
	if card.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Errorf("the fallback card is not labelled deterministic: %s", card.GeneratedBy)
	}
}

func TestAnAbsentLaneIsNotAFailureAndItsCardIsCacheable(t *testing.T) {
	// No lane at all: the deployment says this role runs no model. The floor is
	// the real answer, not a degraded one, and caching it is right.
	s := &Service{now: cardClock}
	f := cardFacts(t)

	card, laneFailed := s.write(context.Background(), f, decideMove(f), project(f, decideMove(f)), true)

	if laneFailed {
		t.Error("a deployment with no lane was reported as a lane failure, so every card it writes would " +
			"be recomputed on every read rather than cached")
	}
	if card.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Errorf("the card is not labelled deterministic: %s", card.GeneratedBy)
	}
}

// Held back by the model-call floor: the facts moved, but too recently to pay
// for another call. Also not a failure — the card is rewritten deterministically
// on purpose, and it must be cached or the floor buys nothing.
func TestACardHeldBackByTheCallFloorIsNotAFailure(t *testing.T) {
	lane := &refusingLane{}
	s := &Service{lane: lane, now: cardClock}
	f := cardFacts(t)

	_, laneFailed := s.write(context.Background(), f, decideMove(f), project(f, decideMove(f)), false)

	if lane.asked {
		t.Fatal("the lane was asked despite the call floor holding the card back")
	}
	if laneFailed {
		t.Error("a card held back by the call floor was reported as a lane failure")
	}
}

// The lane is not asked about a deal with nothing to cite.
//
// This is a refusal to ask rather than a new degrade: with no timeline and no
// open tasks the citable set is empty, keepGrounded drops every sentence for
// want of a citation, and ParseStatus then refuses the empty story. The reply
// was discarded whatever it said, so the call bought a model round-trip and up
// to laneDeadline of the reader's wait to reach the floor already in hand.
//
// The pair is the point: the second half proves the guard is about the CITABLE
// SET and not about deals in general, so a lane that can be used still is.
func TestTheLaneIsNotAskedAboutADealWithNothingToCite(t *testing.T) {
	lane := &refusingLane{}
	s := &Service{lane: lane, now: cardClock}
	f := cardFacts(t)
	f.timeline = nil

	card, laneFailed := s.write(context.Background(), f, decideMove(f), project(f, decideMove(f)), true)

	if lane.asked {
		t.Error("the lane was asked about a deal with nothing to cite, and its answer could only be discarded")
	}
	if laneFailed {
		t.Error("not asking was reported as a lane failure, which would stop the floor being cached")
	}
	if card.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Errorf("the card claims to be %q when no model wrote it", card.GeneratedBy)
	}
}

func TestALaneIsStillAskedWhenThereIsSomethingToCite(t *testing.T) {
	// The admit case. Without it the test above passes against a build that
	// never asks a lane at all.
	lane := &refusingLane{}
	s := &Service{lane: lane, now: cardClock}
	f := cardFacts(t)

	s.write(context.Background(), f, decideMove(f), project(f, decideMove(f)), true)

	if !lane.asked {
		t.Error("a deal with a citable activity did not reach the lane")
	}
}
