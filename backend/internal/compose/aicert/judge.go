// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The judge's call-and-retry drive and the per-run caps gate — what
// runner.go's certifyTask needs beyond the prepared case itself to turn one
// scored answer into one RunResult, split out of runner.go to keep that file
// to the orchestration loop.
//
// The candidate's request is NOT built here, and no longer anywhere in this
// package: each site's own case issues the request its production code
// issues. A scenario's caps.max_tokens therefore grades the answer the model
// gave (checkCaps below); the ceiling the model was handed is the shipped
// builder's, which is the whole point of certifying it.
//
// The judge's own prompt and verdict parse are NOT here either: cert_judge is
// a registered invocation site, and a site's prompt is built in compose
// (compose.JudgeRequest / compose.ParseJudgeVerdict) so the census can
// certify it like every other.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// roleUser is the model.Message role a request's own asks carry. The port
// declares the two-role vocabulary ("user" | "assistant") without exporting a
// constant for either.
const roleUser = "user"

// seededAssistantLabel marks an earlier assistant turn in the grader's copy of
// the ask, so the grader reads it as history rather than as the output.
const seededAssistantLabel = "[Earlier assistant turn, history only, not graded] "

// candidateAsk is the input the grader is shown: the turn the candidate was
// actually handed, read off the first request the case issued.
//
// It is that rather than the corpus fixture because a site's own code builds the
// prompt, and several sites MINT the identifiers they tell the model to answer
// by — a verdict row id, a batch's message ids, a queue's candidate ids —
// precisely so an answer carrying one proves the model read the prompt. Those
// ids exist in the request and nowhere in the fixture, so a grader shown the
// fixture reads every correct, id-bearing answer as invented.
//
// The FIRST request, because that is what the model was ASKED. A site may answer
// in several calls — a shape retry, a fallback, a whole tool loop — and each
// later request is built around a reply that already exists, which is the answer
// under grading rather than the question that produced it.
//
// Every turn of that request, joined, because a site may split one ask across
// several messages (a delimited context block, then the question). An assistant
// turn there is the conversation the scenario seeded, and it is labelled: a
// grader shown only the user turns read "Where are we?" as a question the reply
// ignored when the history had already answered it. The system prompt travels
// separately, as graderInput's product rules.
func candidateAsk(trace aitasks.Trace) (string, error) {
	if len(trace.Requests) == 0 {
		return "", errors.New("the case recorded no request, so there is no input to grade its answer against")
	}
	var turns []string
	asked := false
	for _, m := range trace.Requests[0].Messages {
		if m.Role == roleUser {
			asked = true
			turns = append(turns, m.Content)
			continue
		}
		turns = append(turns, seededAssistantLabel+m.Content)
	}
	if !asked {
		return "", errors.New("the case's first request carries no user turn, so there is no input to grade its answer against")
	}
	return strings.Join(turns, "\n\n"), nil
}

// asGraded is sc as its grader reads it: without the expected answer when the
// site's case declares that answer a checker's specification. A grader shown
// "plain", or the phrases a draft must not use, as "the reference reading"
// grades toward its opposite.
func asGraded(sc Scenario, census *aitasks.Registry) Scenario {
	if factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site); bound && !aitasks.ExpectsReferenceAnswer(factory) {
		sc.Expect.Answer = nil
	}
	return sc
}

// graderInput is everything the grader is shown for one run of sc: the rubric,
// the candidate's first request split into the product rules it was given and
// the ask it answered, the scenario's reference answer, and the output.
//
// The rules come from that same first request, for candidateAsk's reason: they
// are what the site's own code told the model, which the fixture does not hold.
// On an agent loop that System is the whole tool listing, which every opinion
// pays for: the tools' own "use X instead" rules are what the reply is held to.
// The mechanical verdict is not an input: a grader told the answer already
// failed scores that instead of the rubric.
func graderInput(sc Scenario, caseTrace aitasks.Trace, candidateOutput string) (compose.JudgeInput, error) {
	ask, err := candidateAsk(caseTrace)
	if err != nil {
		return compose.JudgeInput{}, err
	}
	return compose.JudgeInput{
		Rubric:          sc.Expect.Rubric,
		ProductRules:    caseTrace.Requests[0].System,
		ScenarioInput:   ask,
		ExpectedAnswer:  string(sc.Expect.Answer),
		CandidateOutput: candidateOutput,
	}, nil
}

// opinion is one judgeVerdict call's reading of a run. graded is false when the
// reply never parsed into a verdict, so score is absent rather than zero.
type opinion struct {
	score       int
	graded      bool
	servedModel string
}

// judgement is what the judge side contributes to one RunResult.
type judgement struct {
	score       int
	scores      []int
	ungraded    bool
	servedModel string
	degraded    bool
}

// judgeScore drives the judge router for one candidate output and folds every
// opinion it asked for into one judgement. A flaky grader never aborts an
// otherwise-healthy certification run: a reply that will not parse is an
// absent opinion, not an error and not a zero.
//
// A run is re-asked where one reading could decide its case: near a bar, or
// anywhere under the floor, since one run under it bars certification. Near is
// both sides of a bar, so a high outlier is re-asked as a low one is.
// wantsAnotherOpinion states the rule; the run scores at the median of the
// opinions that parsed, the mean of two when only two were asked.
//
// The served model is read back from rec's own terminal trace (never
// resp.ServedModel directly) so it carries the same resolved identity the
// candidate side reports, and names the last opinion that was graded. The
// degrade is folded across EVERY call this run made, every opinion and retry
// included: a demotion any of them recovered from still means this run was
// graded on a budget that had run out, which must never be certified silently.
//
// The grader is shown the answer under grading and what graderInput reads off
// the case's own trace: the site's built prompt, never the fixture it was built
// from.
func judgeScore(ctx context.Context, judge *ai.Router, rec *traceRecorder, sc Scenario, caseTrace aitasks.Trace, candidateOutput string, log *slog.Logger) (judgement, error) {
	in, err := graderInput(sc, caseTrace, candidateOutput)
	if err != nil {
		return judgement{}, err
	}
	mark := rec.mark()
	opinions := make([]opinion, 0, maxJudgeOpinions)
	for len(opinions) < maxJudgeOpinions && wantsAnotherOpinion(foldOpinions(opinions).scores, sc.Expect.Bands) {
		next, err := judgeVerdict(ctx, judge, rec, sc.Name, in, log)
		if err != nil {
			return judgement{}, err
		}
		opinions = append(opinions, next)
	}
	calls, err := rec.terminalsSince(mark)
	if err != nil {
		return judgement{}, fmt.Errorf("judge call: %w", err)
	}
	pooled, err := poolRunCalls(calls)
	if err != nil {
		return judgement{}, fmt.Errorf("judge call: %w", err)
	}
	folded := foldOpinions(opinions)
	folded.degraded = pooled.Degraded
	return folded, nil
}

// wantsAnotherOpinion says whether a run holding the graded scores given so far
// is asked again. An opinion that never parsed decides nothing and is replaced,
// within the same maxJudgeOpinions calls.
func wantsAnotherOpinion(given []int, bands Bands) bool {
	switch len(given) {
	case 0:
		return true
	case 1:
		return given[0] < bands.Floor+reaskBandMargin || nearABand(given[0], bands)
	case 2:
		return absDiff(given[0], given[1]) > reaskDisagreement
	default:
		return false
	}
}

// nearABand says score lies within reaskBandMargin of a bar its case is held
// to, where a second reading could move the case across it.
func nearABand(score int, bands Bands) bool {
	for _, bar := range []int{bands.CertifiedMin, bands.DegradedMin, bands.Floor} {
		if absDiff(score, bar) <= reaskBandMargin {
			return true
		}
	}
	return false
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// foldOpinions scores a run at the median of its graded opinions, keeping each
// one in the order it was given. No graded opinion leaves the run ungraded: the
// judge's numbers skip it exactly as they skip a truncated run.
func foldOpinions(opinions []opinion) judgement {
	var folded judgement
	for _, o := range opinions {
		if o.graded {
			folded.scores = append(folded.scores, o.score)
			folded.servedModel = o.servedModel
		}
	}
	if len(folded.scores) == 0 {
		folded.ungraded = true
		return folded
	}
	sorted := slices.Sorted(slices.Values(folded.scores))
	folded.score = medianOf(sorted)
	return folded
}

// judgeVerdict drives one graded call, returning its opinion and the served
// identity of the attempt that opinion came from — the last one the policy
// walked, since that is the reply that was parsed.
func judgeVerdict(ctx context.Context, judge *ai.Router, rec *traceRecorder, scenario string, in compose.JudgeInput, log *slog.Logger) (opinion, error) {
	// Everything in in but the rubric reaches the grader as UNTRUSTED data behind
	// the boundary JudgeRequest mints: the ask is if anything more hostile than
	// the fixture it was built from, because it carries that fixture already
	// wrapped in the candidate site's own markers.
	// The retry is the §5.2 policy rather than a second bare call: a judge that
	// wrapped its JSON in a stray token is TOLD so and can fix it, where the
	// hand-rolled re-ask this replaces showed the second attempt exactly what
	// the first one had already failed on. Each attempt is BUILT again inside
	// CompleteStructured, never re-sent, so JudgeRequest keeps minting the
	// call's data boundary per attempt.
	resp, _, callErr := judge.CompleteStructured(ctx, ai.TaskCertJudge,
		compose.JudgeRequest(in),
		func(text string) error {
			_, err := compose.ParseJudgeVerdict(text)
			return err
		})
	// A withheld or validator-rejected judgement is no opinion, as an unparseable
	// one is, and leaves the run ungraded. Any other failure is the grader's
	// binding breaking, so the run stops and says so.
	if callErr != nil && !ai.ModelDeclined(callErr) {
		return opinion{}, fmt.Errorf("judge call: %w", callErr)
	}
	term, ok := rec.lastTerminal()
	if !ok {
		return opinion{}, fmt.Errorf("judge call: no terminal trace recorded")
	}
	if errors.Is(callErr, model.ErrOutputWithheld) {
		log.WarnContext(ctx, "aicert: the judge's provider withheld its answer — this opinion is left ungraded",
			"scenario", scenario, "err", callErr)
		return opinion{servedModel: term.ServedModel}, nil
	}

	verdict, parseErr := compose.ParseJudgeVerdict(resp.Text)
	if parseErr != nil {
		// A judge that answered the task instead of grading it has no opinion of
		// the candidate, so the candidate is not scored 0 for the judge's failure.
		log.ErrorContext(ctx, "aicert: judge output failed to parse after the validated retry — this opinion is left ungraded",
			"scenario", scenario, "err", parseErr)
		return opinion{servedModel: term.ServedModel}, nil
	}
	return opinion{score: verdict.Score, graded: true, servedModel: term.ServedModel}, nil
}

// selfJudged reports whether the judge shares the candidate's model family — a
// judge grading its own family's output is a weaker signal than an independent
// one, so the record names it rather than hiding it inside an unqualified score.
// An exact match is not required: gemini-3.5-flash grading gemini-3.1-pro-preview
// is a vendor marking its own homework. An empty identity on either side never
// counts — that is a missing trace, not a match. The pre-run refusal (sameModel)
// stays exact on purpose: a same-family judge is allowed, and flagged here.
// bedrockClaude marks a Bedrock model id serving Claude, whatever region prefix
// precedes it.
const bedrockClaude = "anthropic.claude"

func selfJudged(candidateServedModel, judgeServedModel string) bool {
	if candidateServedModel == "" || judgeServedModel == "" {
		return false
	}
	if candidateServedModel == judgeServedModel {
		return true
	}
	candidatePublisher, candidateLine := modelLineage(candidateServedModel)
	judgePublisher, judgeLine := modelLineage(judgeServedModel)
	return (candidatePublisher != "" && candidatePublisher == judgePublisher) ||
		(candidateLine != "" && candidateLine == judgeLine) ||
		publishesLine(candidatePublisher, judgeLine) || publishesLine(judgePublisher, candidateLine)
}

// publishesLine reports whether a broker's publisher is the vendor behind a
// bare model line: "mistralai" publishes the "mistral" a direct provider serves.
func publishesLine(publisher, line string) bool {
	return publisher != "" && line != "" && strings.HasPrefix(publisher, line)
}

// modelLineage splits a served identity into the publisher a broker prefixes it
// with ("mistralai" in mistralai/ministral-8b-2512, empty for a bare name) and
// the model line, the leading letters of the name ("gemini", "gpt" in
// gpt-oss:20b). Two identities agreeing on either are one family. A Bedrock id
// spells the publisher with a dot and a region before it
// (us.anthropic.claude-…), so its Claude is read as anthropic's claude.
func modelLineage(servedModel string) (publisher, line string) {
	name := servedModel
	if slash := strings.LastIndex(servedModel, "/"); slash >= 0 {
		publisher, name = strings.ToLower(servedModel[:slash]), servedModel[slash+1:]
	}
	if at := strings.Index(strings.ToLower(name), bedrockClaude); at >= 0 {
		publisher, name = "anthropic", name[at+len("anthropic."):]
	}
	end := strings.IndexFunc(name, func(r rune) bool { return !unicode.IsLetter(r) })
	if end < 0 {
		end = len(name)
	}
	return publisher, strings.ToLower(name[:end])
}

// cloudServed reports whether provider names a network-hosted vendor, so
// the scenario's P95 latency cap only ever judges a call whose latency
// reflects a real network round-trip, never a same-host inference
// engine's hardware (spec: "Caps.P95LatencyMS applies to cloud-served
// candidates only"). Delegates to ai.ProviderIsLocal rather than
// re-encoding that set here — a second copy could drift from the one
// ai's own conformance test binds.
func cloudServed(provider string) bool {
	return !ai.ProviderIsLocal(provider)
}

// checkCaps reports whether a run's usage stays within sc's resource
// ceilings, alongside a human-readable reason per breach — a run over
// cap fails HardPass exactly like a failed structural check, never
// silently.
//
// The ceilings govern the RUN, so they are read off its pooled calls: a site
// that answers in three requests spent all three, and a cap charged to the last
// one alone would pass a run that blew its budget twice over on the way there.
func checkCaps(caps Caps, run runCalls) (ok bool, failures []string) {
	if caps.MaxTokens > 0 {
		// caps.max_tokens budgets the model's ANSWER — the reply it
		// generates — never the scenario's fixed input (which the model
		// cannot shrink) nor the internal thinking a reasoning model spends
		// before answering (that thinking is not the answer the cap governs;
		// see runMaxOutputTokens and ai.ReasoningOutputMaxTokens). Grade the
		// answer alone, so a rich-input scenario with a tight OUTPUT cap
		// tests what it means to — did the model draft within budget — rather
		// than failing on input size a bigger prompt would always blow.
		answer := run.TokensOut - run.ReasoningTokens
		if answer > caps.MaxTokens {
			failures = append(failures, fmt.Sprintf("max_tokens cap %d exceeded: %d answer tokens", caps.MaxTokens, answer))
		}
	}
	if caps.P95LatencyMS > 0 && cloudServed(run.Provider) && run.LatencyMS > caps.P95LatencyMS {
		failures = append(failures, fmt.Sprintf("p95_latency_ms cap %d exceeded: %dms", caps.P95LatencyMS, run.LatencyMS))
	}
	return len(failures) == 0, failures
}
