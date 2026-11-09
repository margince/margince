// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Replaying a journaled run through the code that runs now. The journal keeps
// what the models said, never what this package concluded from it. So a
// validator or caps change reaches every replayed run.
//
// The case is driven again with a completer that answers from the journal. Its
// requests must match the journaled ones call for call, up to the per-call row
// ids each preparation mints. Those ids are renamed in the answers, so the case
// reads its own.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// runLane is one worker's candidate and judge routers with their recorders. A
// recorder answers "what did this run call" by position, so two runs in flight
// never share one.
type runLane struct {
	candidate    *ai.Router
	candidateRec *traceRecorder
	judge        *ai.Router
	judgeRec     *traceRecorder
}

// measuredRun is one scored run and what a later run needs to replay it; a
// nil evidence leaves the run unreplayable.
type measuredRun struct {
	outcome  runOutcome
	evidence *replayEvidence
}

// replayEvidence is what the models said in one run, and what the caps read of
// its calls, which the journal does not otherwise hold.
type replayEvidence struct {
	// Routing is the replayRouting the run was served under.
	Routing string         `json:"routing"`
	Calls   []replayedCall `json:"calls"`
	// IDs are the per-call row ids the run's requests carried, by first appearance.
	IDs             []string `json:"ids,omitempty"`
	ReasoningTokens int      `json:"reasoning_tokens,omitempty"`
	Truncated       bool     `json:"truncated,omitempty"`
	// GraderDigest is the canonical digest of the request the judge graded,
	// empty when no judge was asked.
	GraderDigest string `json:"grader_digest,omitempty"`
}

// replayedCall is one candidate call: the shape of what was asked, and the answer.
type replayedCall struct {
	Shape    string         `json:"shape"`
	Response model.Response `json:"response"`
}

// recordingCompleter passes a live run's calls through and keeps each answer.
type recordingCompleter struct {
	next      aitasks.Completer
	mu        sync.Mutex
	exchanges []exchange
	// failed marks a call that errored. A case that carried on past it cannot be
	// replayed, because the journal has no faithful form for an error.
	failed bool
}

type exchange struct {
	request  model.Request
	response model.Response
}

func (c *recordingCompleter) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	resp, err := c.next.Complete(ctx, req)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.failed = true
		return resp, err
	}
	c.exchanges = append(c.exchanges, exchange{request: req, response: resp})
	return resp, nil
}

// replayable says the run's calls all answered, so the journal can hold them.
func (c *recordingCompleter) replayable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.failed && len(c.exchanges) > 0
}

// evidence is what the journal keeps of a replayable run.
func (c *recordingCompleter) evidence(pooled runCalls) (*replayEvidence, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var table idTable
	ev := &replayEvidence{Routing: replayRouting, ReasoningTokens: pooled.ReasoningTokens, Truncated: pooled.Truncated}
	for _, x := range c.exchanges {
		shape, err := requestShape(x.request, &table)
		if err != nil {
			return nil, err
		}
		ev.Calls = append(ev.Calls, replayedCall{Shape: shape, Response: x.response})
	}
	ev.IDs = table.ids
	return ev, nil
}

// replayCompleter answers a case from a journaled run, refusing any request
// whose shape differs from the one the journaled answer was given to.
type replayCompleter struct {
	evidence *replayEvidence
	mu       sync.Mutex
	table    idTable
	served   int
	diverged error
}

func (c *replayCompleter) Complete(_ context.Context, req model.Request) (model.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.served >= len(c.evidence.Calls) {
		c.diverged = fmt.Errorf("the case asks more than the %d call(s) the journaled run made", len(c.evidence.Calls))
		return model.Response{}, c.diverged
	}
	call := c.evidence.Calls[c.served]
	c.served++
	shape, err := requestShape(req, &c.table)
	if err != nil {
		c.diverged = err
		return model.Response{}, err
	}
	if shape != call.Shape {
		c.diverged = fmt.Errorf("call %d is not the request the journaled answer was given to", c.served)
		return model.Response{}, c.diverged
	}
	resp := call.Response
	resp.Text = perCallID.ReplaceAllStringFunc(resp.Text, c.rename)
	return resp, nil
}

// rename maps an id the journaled run minted to the one this preparation
// minted in the same place. An id no request carried is the model's own and stays.
func (c *replayCompleter) rename(id string) string {
	at := slices.IndexFunc(c.evidence.IDs, func(minted string) bool { return strings.EqualFold(minted, id) })
	if at < 0 || at >= len(c.table.ids) {
		return id
	}
	return c.table.ids[at]
}

// finished says whether the case consumed the journaled calls, no more and no fewer.
func (c *replayCompleter) finished(runErr error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.diverged != nil:
		return c.diverged
	case runErr != nil:
		return fmt.Errorf("the case failed on the journaled answers: %w", runErr)
	case c.served < len(c.evidence.Calls):
		return fmt.Errorf("the case made %d of the %d calls the journaled run made", c.served, len(c.evidence.Calls))
	}
	return nil
}

// errNoEvidence is a journaled answer with nothing to replay it from.
var errNoEvidence = errors.New("the journaled run carries no replay evidence")

// reevaluate is a journaled run as the code that runs now grades it. The
// judge's opinion is kept only when the judge would be shown the same request.
//
// A run with no answer, withheld or broken off, is kept whole. No code grades
// it, so only a change to the routing can move it.
func reevaluate(ctx context.Context, census *aitasks.Registry, task ai.Task, sc Scenario, line journaledRun, log *slog.Logger) (runOutcome, error) {
	kept, ev := line.Outcome, line.Evidence
	switch {
	case ev == nil:
		return runOutcome{}, errNoEvidence
	case ev.Routing != replayRouting:
		return runOutcome{}, fmt.Errorf("the run was served under %q, and this build routes as %q", ev.Routing, replayRouting)
	case !kept.delivered():
		return kept, nil
	}
	bound, err := prepareCase(census, task, sc)
	if err != nil {
		return runOutcome{}, err
	}
	replay := &replayCompleter{evidence: ev}
	caseTrace, runErr := bound.prepared.Run(ctx, replay)
	if err := replay.finished(runErr); err != nil {
		return runOutcome{}, err
	}
	pooled := runCalls{
		Provider: kept.Provider, ServedModel: kept.ServedModel, ServedIdentitySource: kept.ServedIdentitySource,
		TokensIn: kept.TokensIn, TokensOut: kept.TokensOut, CachedTokens: kept.CachedTokens,
		CacheWriteTokens: kept.CacheWriteTokens, ReasoningTokens: ev.ReasoningTokens,
		LatencyMS: kept.LatencyMS, Truncated: ev.Truncated,
	}
	scored, err := scoreAnswer(ctx, bound.prepared, bound.scope, caseTrace, sc, task, pooled, log)
	if err != nil {
		return runOutcome{}, err
	}
	// Whether a route was degraded is a fact about the calls, not the grading,
	// so the degrade gate reads the journaled one.
	scored.outcome.Degraded = kept.Degraded
	if !scored.judged {
		return scored.outcome, nil
	}
	digest, err := graderDigest(asGraded(sc, census), caseTrace, scored.output)
	if err != nil {
		return runOutcome{}, err
	}
	if digest != ev.GraderDigest {
		return runOutcome{}, errors.New("the judge would be shown a request other than the one it graded")
	}
	out := scored.outcome
	out.Score, out.JudgeScores, out.Ungraded = kept.Score, kept.JudgeScores, kept.Ungraded
	out.JudgeServedModel, out.JudgeDegraded = kept.JudgeServedModel, kept.JudgeDegraded
	return out, nil
}

// graderDigest is the canonical digest of the request the judge is shown for
// one answer.
func graderDigest(graded Scenario, caseTrace aitasks.Trace, output string) (string, error) {
	in, err := graderInput(graded, caseTrace, output)
	if err != nil {
		return "", err
	}
	return canonicalRequestDigest(compose.JudgeRequest(in))
}
