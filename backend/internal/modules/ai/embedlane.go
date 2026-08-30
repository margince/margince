// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The embedding lane: metered like a chat call, routed to the one embed
// binding — split from router.go on the file-length cap.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Embed routes the embedding lane. Inputs are stripped before egress —
// the EmbedRequest carries no per-request hook, so the router is the
// enforcement point here. One provider call is exactly one logical call —
// the embed lane has no retry ladder to bundle.
func (r *Router) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	// One load per call — see binding: a rebind mid-call must not mix an embedder
	// with another binding's width or provider label.
	b := r.binding()
	if _, ok := principal.WorkspaceID(ctx); !ok {
		return model.Embeddings{}, fmt.Errorf("ai: embeddings outside workspace context")
	}
	stripped := make([]string, len(req.Inputs))
	for i, input := range req.Inputs {
		clean, _, err := r.stripper.Strip(ctx, []byte(input))
		if err != nil {
			return model.Embeddings{}, fmt.Errorf("ai: stripping embed input: %w", err)
		}
		stripped[i] = string(clean)
	}
	req.Inputs = stripped
	if req.Dimensions == 0 {
		// The configured embeddings binding's width (defaulted/validated
		// once by ParseRouting) is the operator's choice — a caller that
		// names no explicit width gets that configured one, never a
		// silent per-adapter default the operator never set.
		req.Dimensions = b.embedDims
	}

	// One embedding is one forward pass and answers in about a second, so a
	// minute of silence is a connection that will not answer rather than a model
	// still working. Without this the caller waits out requestTimeout — five
	// minutes, with its database transaction open — and a re-embed pass spends
	// its River attempts on connections the network already dropped.
	//
	// A deadline the caller set EARLIER still wins: WithTimeout only ever
	// shortens, so a request that is already nearly out of time is not handed a
	// fresh minute here.
	embedCtx, cancel := context.WithTimeout(ctx, EmbedCallTimeout)
	defer cancel()

	start := r.now()
	res, err := b.embedder.Embed(embedCtx, req)
	trace := Call{Task: TaskEmbeddings, Tier: TierEmbedLane, Kind: callKindEmbedding, CacheOff: r.cacheOff, LatencyMS: r.now().Sub(start).Milliseconds()}
	if err == nil {
		// Stamp the SAME token estimate the meter records below onto the
		// trace row too — embeddings are input-only (no output, no cache
		// buckets), so TokensOut/cache fields stay 0. Without this, ai_call
		// carries tokens_in=0 while ai_usage carries the real estimate for
		// the identical call; CostReport treats a zero-usage row as free by
		// construction (a call that failed before reaching the provider),
		// so a paid embedding model priced to a silent $0 despite a
		// nonzero token line — cost is transparency, never a silent 0. A
		// failed call (err != nil) legitimately never reached the
		// provider, so it keeps TokensIn at 0 and reads free, same as today.
		trace.TokensIn = embedTokenEstimate(req.Inputs)
	}
	if cid, ok := principal.CorrelationID(ctx); ok {
		trace.CorrelationID = &cid
	}
	if m, ok := b.routeMeta[TierEmbedLane]; ok {
		trace.Provider, trace.ModelID = m.provider, m.model
	}
	trace.ErrorSentinel = classifyError(err)
	// model.Embeddings carries no served-model identity (no adapter reports
	// one for the embed lane today), so this always falls back to the
	// tier's configured binding.
	trace.ServedModel, trace.ServedIdentitySource = servedIdentity(trace.Provider, trace.ModelID, "")
	if r.CapturesPayload(TaskEmbeddings) && trace.ErrorSentinel == "" {
		trace.Payload = r.buildEmbedPayload(req, res)
	}
	lc := newLogicalCall()
	lc.append(trace)
	r.flushDetached(ctx, b, lc)
	if err != nil {
		return model.Embeddings{}, err
	}
	// The embed lane spends the workspace budget like any other call, so it
	// spends the agent's share of it too. A retrieval-heavy agent whose
	// embeddings were free would be the one shape this counter never sees.
	r.spendAgentTokens(ctx, trace.TokensIn)
	if err := r.meter.Record(ctx, Usage{Task: TaskEmbeddings, Tier: TierEmbedLane, TokensIn: trace.TokensIn}); err != nil {
		return model.Embeddings{}, fmt.Errorf("ai: call served but metering failed: %w", err)
	}
	return res, nil
}

// EmbedIdentity names the current embed binding for search to stamp on
// every row and filter retrieval to (search.Embedder) — cheap, no API
// call. Returns ("", 0) when the embed lane is unbound (--ai-fake with no
// embeddings configured, or any boot that never bound one): routeMeta
// only carries a TierEmbedLane entry when routing_bind.go's
// embedInclusiveMeta saw a non-empty Embeddings.Model, so a missing entry
// here is the honest "nothing to identify" case, never a panic on a
// missing map key.
func (r *Router) EmbedIdentity() (string, int) {
	// One load: the identity and the width it names must come from the same
	// binding, or the string reports a model at a width it was never asked for.
	b := r.binding()
	m, ok := b.routeMeta[TierEmbedLane]
	if !ok {
		return "", 0
	}
	return fmt.Sprintf("%s/%s@%d", m.provider, m.model, b.embedDims), b.embedDims
}
