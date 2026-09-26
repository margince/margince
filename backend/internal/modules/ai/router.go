// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Embedding lane labels for metering: not a chat tier (§1.1).
const (
	TaskEmbeddings Task = "embeddings"
	TierEmbedLane  Tier = "embed"
)

// resultCacheTTL bounds staleness for cached completions (§6: TTL and
// record-change invalidation; the workspace-scoped Invalidate hook is
// the latter's entry point).
const resultCacheTTL = 15 * time.Minute

// traceWriteTimeout bounds the deferred ai_call write that runs on a
// context detached from the request's cancellation.
const traceWriteTimeout = 5 * time.Second

// routeMeta is the provider/model identity per tier, retained from the
// routing config so every ai_call row and RouteInfo can name what served
// the call without reaching into the opaque Client.
type routeMeta struct {
	provider string
	model    string
	// baseURL is part of the identity rejectedAgainAbove compares: one model
	// behind two endpoints is two APIs, which may refuse different requests.
	baseURL string
}

// Router is the tiered routing engine (B-EP06.4): tasks name tiers,
// tiers resolve to bound Clients, the budget guardrail bends the route
// before the call, and every call lands in the meter. This is the one
// place routing policy lives — callers never pick a model.
type Router struct {
	// bound is the current binding. Load it once per call — see the type's
	// comment for why a second load mid-call is a correctness bug, not a
	// performance one.
	bound atomic.Pointer[binding]
	// generations mints the binding identity install stamps. Monotonic for
	// the life of the Router, never reset — a reused generation would make a
	// stale cached answer readable again.
	generations atomic.Uint64
	meter       usageStore
	// agentSpend is the per-Passport share of the workspace budget
	// (MCP-SESS-COST). Nil in every role that serves no inbound agent.
	agentSpend      AgentTokenSpender
	budget          BudgetPolicy
	stripper        model.SecretStripper
	cache           *resultCache
	calls           callStore
	capturePayloads bool
	log             *slog.Logger
	metrics         *callMetrics
	now             func() time.Time
	// cacheOff disables the §6 result cache entirely (ai.WithoutResultCache):
	// the cert lane and scripted repeat-call tests need every call to reach
	// the model, not collapse onto a cached answer.
	cacheOff bool
	// decisionCertified answers whether the decisions lane may serve one site:
	// the generated certification table, except on the certification lane's
	// own DB-less router (WithEveryDecisionCertified).
	decisionCertified func(DecisionCertKey) bool
	// decisionTimeout bounds one decision call (DecisionCallTimeout).
	decisionTimeout time.Duration
}

// installConfigSnapshot computes and stores this Router's config-snapshot
// dimension row from the routing yaml's digest (RoutingConfig.sourceHash)
// and the configured embed-lane width, and stamps embedDims onto the Router
// itself — the one call that keeps both in sync, since the snapshot's
// provider_params must name the SAME width Embed defaults an unset request
// to. Pure — no DB access; EnsureConfig plants the row lazily, once per
// flush.

// RouteInfo tells the caller how its request was actually served — the
// honest "reduced quality" signal the UI surfaces in economy mode, plus
// the provider/model identity the agent-run trace records (RUNNER-AC-4).
type RouteInfo struct {
	Tier     Tier
	Provider string
	ModelID  string
	Degraded bool
	Cached   bool
}

// NewRouter builds the production router from a validated routing config.
// calls traces every completion terminal (ai_call); capturePayloads gates
// the Layer-3 content capture; log carries router observability.
func NewRouter(cfg RoutingConfig, meter *Meter, budget BudgetPolicy, calls callStore, capturePayloads bool, log *slog.Logger) (*Router, error) {
	clients, embedder, err := cfg.buildClients()
	if err != nil {
		return nil, err
	}
	decisions, err := cfg.buildDecisionLane()
	if err != nil {
		return nil, err
	}
	meta := embedInclusiveMeta(cfg)
	router := assembleRouter(clients, embedder, cfg.Profile, meter, budget, calls, meta, capturePayloads, log)
	router.install(router.binding().withConfig(cfg, decisions))
	return router, nil
}

// assembleRouter is the seam unit tests inject fakes through.
func assembleRouter(clients map[Tier]model.Client, embedder model.Client, profile Profile, meter usageStore, budget BudgetPolicy, calls callStore, meta map[Tier]routeMeta, capturePayloads bool, log *slog.Logger) *Router {
	if log == nil {
		log = slog.Default()
	}
	r := &Router{
		meter:           meter,
		budget:          budget,
		stripper:        NewSecretStripper(),
		cache:           newResultCache(resultCacheTTL),
		calls:           calls,
		capturePayloads: capturePayloads,
		log:             log,
		// Every Router shares the one process-wide collector (metrics.go):
		// coldStartOptions and offerDraftOptions each mint their own Router
		// over the same routing config, and /metrics must report one honest
		// total across both, rendered exactly once.
		metrics:           sharedCallMetrics,
		now:               time.Now,
		decisionCertified: decisionIsCertified,
		decisionTimeout:   DecisionCallTimeout,
	}
	r.install(binding{clients: clients, embedder: embedder, profile: profile, routeMeta: meta})
	return r
}

// Complete routes one task to a completion. The request names no model:
// the resolved tier's binding supplies it. It is exactly one logical
// call — mint, attempt, flush.
func (r *Router) Complete(ctx context.Context, task Task, req model.Request) (model.Response, RouteInfo, error) {
	ladder, ok := taskLadders[task]
	if !ok {
		return model.Response{}, RouteInfo{}, fmt.Errorf("ai: unknown task %q", task)
	}
	return r.serveCompletion(ctx, task, ladder, req)
}

// serveCompletion serves one call over an explicit ladder as its OWN
// logical call — the seam router unit tests drive directly, and the
// convenience wrapper Complete uses for its single-attempt case.
func (r *Router) serveCompletion(ctx context.Context, task Task, ladder []Tier, req model.Request) (model.Response, RouteInfo, error) {
	lc := newLogicalCall()
	// DEFERRED, like CompleteStructured's. A panic in a provider adapter, the
	// cache or the meter unwinds through finalizeAttempt — which appends the
	// terminal trace to lc — and would then skip a plain statement here, so the
	// trace would be built and never written. That was survivable while the
	// router only ever announced a settled occurrence; it stopped being
	// survivable when it began announcing a start, because the start is
	// committed durable state that only this flush closes.
	//
	// The binding is loaded HERE and not carried out of serveAttempt, so a
	// rebind during the call files the trace under the configuration current at
	// flush rather than the one that served it. That is pre-existing and
	// unchanged by this defer — serveAttempt owns its own `b` and does not
	// publish it — but the comment that used to sit here claimed the opposite,
	// and a false claim about which configuration a trace names is worse than
	// the gap it hid.
	defer func() { r.flushDetached(ctx, r.binding(), lc) }()
	return r.serveAttempt(ctx, lc, task, ladder, req, "")
}

// serveAttempt serves ONE attempt over ladder and appends its trace to lc
// — it never flushes. CompleteStructured (structured.go) calls this
// directly, threading one shared lc across the whole retry/escalation
// chain so every rung the caller's request actually walked lands under
// one LogicalCallID; serveCompletion wraps it for the single-attempt case.
func (r *Router) serveAttempt(ctx context.Context, lc *logicalCall, task Task, ladder []Tier, req model.Request, reason string) (resp model.Response, info RouteInfo, err error) {
	// One load per attempt. Every tier resolution, provider label and config hash
	// below comes from THIS binding, so the call lands in the meter under the
	// configuration that actually served it.
	b := r.binding()
	rawWS, ok := principal.WorkspaceID(ctx)
	if !ok {
		// No workspace ⇒ no RLS-writable trace row; fail before building
		// the trace so we never attempt a tenant write outside a tenant.
		return model.Response{}, RouteInfo{}, fmt.Errorf("ai: task %s outside workspace context", task)
	}
	wsID := ids.From[ids.WorkspaceKind](rawWS)
	ladder, degraded, budgetErr := r.applyBudget(ctx, task, wsID, ladder)
	if budgetErr != nil && errors.Is(budgetErr, ErrBudgetDeferred) {
		// A deferral is pacing, not failure — the work re-queues itself for
		// the boundary the error names — so it is neither traced nor
		// announced; a broken budget READ falls through to the traced
		// return below.
		return model.Response{}, RouteInfo{}, budgetErr
	}
	if req.SecretStripper == nil {
		req.SecretStripper = r.stripper
	}
	// Wrapped HERE, which is the one place a request's stripper is settled —
	// so what it removed reaches the trace row without four adapters each
	// having to thread a report back, and a fifth records its strips by
	// existing. striprecorder.go says why that matters.
	strips := newStripRecorder(req.SecretStripper)
	req.SecretStripper = strips
	key, keyErr := cacheKey(wsID, task, req)
	if keyErr == nil {
		// The site's own defect, found with the key's: before any provider.
		req, keyErr = withSiteThinking(req, task)
	}

	// Every terminal from here on is traced — the budget-read and cache-key
	// failures included: one Call appended to lc for the served call, the
	// cache hit, or the failure, and the settle announce needs no start
	// line to have run. Only the no-workspace return above is untraced:
	// with no tenant there is no row to write and no occurrence to
	// announce.
	start := r.now()
	trace := r.newAttemptTrace(ctx, task, key, reason, req)
	defer func() {
		// BEFORE finalize, which is what buffers the row: a field set after it
		// would be written to a copy nobody reads.
		trace.SecretsRemoved, trace.SecretKinds = strips.report()
		r.finalizeAttempt(ctx, b, lc, &trace, req, resp, err, start)
	}()
	if budgetErr != nil {
		return model.Response{}, RouteInfo{}, budgetErr
	}
	if keyErr != nil {
		// No provider was tried: the sentinel must say so, or the trace
		// blames a vendor for our own key construction.
		return model.Response{}, RouteInfo{}, fmt.Errorf("%w: %w", errRequestFailed, keyErr)
	}

	trace.Degraded = degraded
	if degraded && !isDecisionFallbackReason(reason) {
		// The budget guardrail forced a demoted ladder — worth naming on the
		// walk's first rung even when that is otherwise attempt 1, since it
		// explains why this walk did not run the caller's default route. A decision
		// attempt's reason is kept: it says why the ladder ran at all, and
		// Degraded still says the band demoted it.
		trace.AttemptReason = attemptReasonBudgetDegrade
	}
	ladder = servableLadder(b, task, ladder)
	if len(ladder) == 0 {
		return model.Response{}, RouteInfo{}, localOnlyRefusal(task, b.routeMeta)
	}

	// The rail's opening line. It sits HERE and not higher — announceRailStartOnce
	// says why.
	lc.announceRailStartOnce(ctx, r, task)

	// A cached answer only serves when its tier is still on the adjusted
	// ladder: after a budget band tightened or the profile remapped the
	// route, a premium-tier entry must not smuggle premium output into an
	// economy route. The stale entry stays put — TTL ages it out, and the
	// band may relax within its lifetime. A cache-off Router (§ cert lane,
	// scripted repeat-call tests) never consults it: every call must reach
	// the model.
	if cached, tier, hit := r.cache.get(key, wsID, b.generation); !r.cacheOff && hit && tierOnLadder(ladder, tier) {
		return r.serveCacheHit(ctx, b, &trace, task, tier, cached, degraded)
	}

	out, tier, served, ladderErr := r.attemptLadder(ctx, b, lc, &trace, task, ladder, req, key, wsID, start)
	// Stamp tier and usage even when the ladder returns an error: a
	// metering failure of a successfully-served call still spent provider
	// tokens on a real tier, and an all-rungs-failed walk names the last
	// tier attempted — the trace records what actually happened, not an
	// empty terminal.
	trace.Tier = tier
	trace.TokensIn, trace.TokensOut = out.InputTokens, out.OutputTokens
	trace.ReasoningTokens, trace.CachedTokens = out.ReasoningTokens, out.CachedTokens
	trace.CacheWriteTokens = out.CacheWriteTokens
	if ladderErr != nil {
		return model.Response{}, RouteInfo{}, ladderErr
	}
	if served {
		m := b.routeMeta[tier]
		return out, RouteInfo{Tier: tier, Provider: m.provider, ModelID: m.model, Degraded: degraded}, nil
	}
	// The honest degraded state (§4.3): no bound model can serve this.
	//
	// ErrAllTiersFailed, the same as a walk that tried every rung and got
	// nothing: from a caller's seat the two are one fact — no model answered —
	// and the distinction that matters to them is against a request that was
	// wrong. An installation with NOTHING bound is the commonest way to reach
	// this, so leaving it unmarked is leaving the sentinel off the case a first
	// run actually hits.
	return model.Response{}, RouteInfo{},
		fmt.Errorf("%w: no bound tier can serve %s in profile %s", ErrAllTiersFailed, task, b.profile)
}

// servableLadder is the ladder a call over b may actually walk: remapped for
// the sovereign profile, then narrowed to same-host rungs for a local-only
// task. Profile and clients come from the one binding snapshot, so no ladder
// mixes two loads. serveAttempt walks it and Decide peeks the cache against
// it, so both read the same ladder when asking which cached answer would serve.
func servableLadder(b *binding, task Task, ladder []Tier) []Tier {
	_, hasLarge := b.clients[TierLocalLarge]
	return localOnlyLadder(task, b.routeMeta, profileLadder(b.profile, hasLarge, ladder))
}

// Invalidate drops a workspace's cached results — the hook the §6
// record-change invalidation rides (wired from event consumers).
func (r *Router) Invalidate(workspaceID ids.WorkspaceID) { r.cache.invalidate(workspaceID) }

// applyBudget bends the ladder per §1.3: soft-degrade one tier at 80%,
// defer background work / pin interactive work to local-small at 100%.
func (r *Router) applyBudget(ctx context.Context, task Task, wsID ids.WorkspaceID, ladder []Tier) ([]Tier, bool, error) {
	budgetTokens, err := r.budget.MonthlyTokenBudget(ctx, wsID)
	if err != nil {
		return nil, false, fmt.Errorf("ai: budget policy: %w", errors.Join(errBudgetUnavailable, err))
	}
	if budgetTokens <= 0 {
		// Fail closed on misconfiguration — an accidental zero budget must
		// not read as "unlimited".
		return nil, false, fmt.Errorf("ai: workspace has a non-positive token budget (%d): %w", budgetTokens, errBudgetUnavailable)
	}
	spent, err := r.meter.MonthTokens(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("ai: reading month usage: %w", errors.Join(errBudgetUnavailable, err))
	}
	return budgetLadder(task, ladder, BudgetBand(spent, budgetTokens), r.now().UTC())
}

// localTiers are the rungs that run on the box. The set is written this way
// round — naming what is SAFE rather than what egresses — so that a tier added
// to the contract and forgotten here is remapped rather than let through:
// an incomplete allowlist of cloud rungs would fail open under the one profile
// that promises it never will. TestEveryLocallyNamedTierIsClassifiedLocal
// keeps the two spellings of "local" agreeing.
var localTiers = map[Tier]bool{
	TierLocalSmall: true,
	TierLocalLarge: true,
}

// costlyCloudTiers are the rungs billed above the cheap cloud rate. The
// §1.3 routing-fix alarm measures their combined share, so a rung priced above
// premium belongs here the day it is declared — an alarm that watches only
// part of the expensive spend reads LOWER the more of it a workspace does.
var costlyCloudTiers = []Tier{TierPremium, TierFrontier}

// embedTokenEstimate meters the embed lane by the ~4-bytes-per-token
// heuristic; local embedders report no usage counts.
func embedTokenEstimate(inputs []string) int {
	total := 0
	for _, s := range inputs {
		total += len(s) / 4
	}
	return total
}

// cacheKey covers EVERY completion-shaping input (model override, system,
// messages, tools, max tokens, response schema, attachments, provider
// options, and company-context binding) via a collision-resistant digest,
// prefixed with the plaintext workspace id: a hash collision may spoil a cache
// hit but can never cross a tenant boundary, because the workspace segment is
// compared literally (and re-checked against the stored entry on read).
// Attachments and provider options MUST be in the digest — otherwise two calls
// with identical prompt text but a different attached document (or a different
// reasoning/thinking knob) collide, and the second is served the first's answer.
func cacheKey(wsID ids.WorkspaceID, task Task, req model.Request) (string, error) {
	req = withCanonicalFence(req)
	material, err := json.Marshal(struct {
		Model              string                     `json:"model"`
		System             string                     `json:"system"`
		Messages           []model.Message            `json:"messages"`
		Tools              []model.ToolDef            `json:"tools"`
		MaxTokens          int                        `json:"max_tokens"`
		ResponseSchema     json.RawMessage            `json:"response_schema"`
		Attachments        []model.Attachment         `json:"attachments"`
		ProviderOptions    map[string]json.RawMessage `json:"provider_options"`
		ContextScopes      []string                   `json:"context_scopes"`
		ContextFingerprint string                     `json:"context_fingerprint"`
		Site               string                     `json:"site"`
		ThinkingFloor      string                     `json:"thinking_floor,omitempty"`
	}{req.Model, req.System, req.Messages, req.Tools, req.MaxTokens, req.ResponseSchema, req.Attachments, req.ProviderOptions, req.ContextScopes, req.ContextFingerprint, req.Site, req.ThinkingFloor})
	if err != nil {
		// A ProviderOptions namespace carrying invalid JSON would otherwise
		// marshal to nil and collapse every such request onto one cache key —
		// fail loudly instead of serving a collided answer.
		return "", fmt.Errorf("ai: cache key: %w", err)
	}
	sum := sha256.Sum256(material)
	return wsID.String() + "|" + string(task) + "|" + hex.EncodeToString(sum[:]), nil
}

// withCanonicalFence returns req with its data boundary replaced by a fixed
// placeholder, for hashing only — the returned request is never sent.
//
// Prompts that carry captured text bound it with a marker minted per call
// (shared/kernel/promptfence), which is the point: nothing the sender writes can
// close it. But a fresh marker in every prompt is also a fresh cache key for
// every prompt, and the cache would never hit again — worse than a wasted
// optimization, since capture's auto-enrich extracts a SENDER'S site, so
// repeated mail from one domain would pay a fresh model call each time instead
// of collapsing onto one cached extraction.
//
// Only the marker the SYSTEM prompt declares is replaced, and only literally.
// The system prompt is text this codebase wrote, so a hostile page cannot choose
// which string is treated as the boundary and cannot make two different payloads
// share a key.
func withCanonicalFence(req model.Request) model.Request {
	declaring := req.System
	req.System = promptfence.Canonicalize(declaring, declaring)
	messages := make([]model.Message, len(req.Messages))
	for i, m := range req.Messages {
		m.Content = promptfence.Canonicalize(declaring, m.Content)
		messages[i] = m
	}
	req.Messages = messages
	return req
}
