// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Running the brief's two writers.
//
// Its own file because it is its own concept, and because service.go had grown
// past the point where a reader can hold it. What is here is the ONE decision:
// the sections and the plan are written at the same time, from the same lane,
// and neither can take the other down.

import (
	"context"
	"log/slog"
	"sync"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// writtenBrief is what the two writers produced, and which writer produced it.
type writtenBrief struct {
	sections   []Section
	sectionsBy crmcontracts.WrittenBy
	plan       Plan
	planBy     crmcontracts.WrittenBy
}

// write produces both halves of the brief.
func (s *Service) write(ctx context.Context, in Input) writtenBrief {
	// The two halves are written CONCURRENTLY. They share a lane and nothing
	// else: the sections say what is known, the plan says what to do, and
	// running them in sequence would make a reader wait for the first before
	// the second even started. Each degrades to its own floor, so one slow or
	// failing call cannot take the other down.
	//
	// The plan gets its OWN ranked claim set rather than the one the sections
	// consumed. `take` marks a claim as spoken so no section repeats another,
	// and the plan is not a tenth section: it is the same facts read for what
	// to DO about them, so the promise the goal names is also the promise the
	// objective aims at. Sharing one set would have the plan silently skip
	// whatever the sections had already used.
	lang := identity.BaseLanguageForPrompt(ctx, s.pool)
	floor := DeterministicPlan(in, rankClaims(in))
	var (
		out  writtenBrief
		both sync.WaitGroup
	)
	both.Add(2)
	go func() {
		defer both.Done()
		// A panic in a goroutine takes the PROCESS down, not the request. The
		// whole posture of both writers is that a model failure degrades to
		// the floor, and a nil map or a bad index in a rewrite path would
		// otherwise be the one model failure that ends the server.
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, "meeting brief: the sections writer panicked; serving the deterministic floor", "panic", recovered)
				out.sections, out.sectionsBy = Deterministic(in), crmcontracts.Deterministic
			}
		}()
		out.sections, out.sectionsBy = Write(ctx, s.lane, in, lang)
	}()
	go func() {
		defer both.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, "meeting brief: the plan writer panicked; serving the deterministic floor", "panic", recovered)
				out.plan, out.planBy = floor, crmcontracts.Deterministic
			}
		}()
		out.plan, out.planBy = WritePlan(ctx, s.lane, in, floor, lang)
	}()
	both.Wait()
	return out
}
