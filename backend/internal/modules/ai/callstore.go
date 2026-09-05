// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// callKindCompletion and callKindEmbedding are the ai_call.kind vocabulary
// (0100): a chat-ladder call versus one embed-lane call. Every Call this
// package builds sets one explicitly — the column carries NOT NULL with a
// CHECK, so an unset Kind would reach the database as an empty string and
// fail the constraint, not fall back to the SQL-side DEFAULT.
const (
	callKindCompletion = "completion"
	callKindEmbedding  = "embedding"
)

// Call is one ATTEMPT's trace record (Layer 1, spec §4): who routed
// where, how many tokens it burned, and whether it was served, cached, or
// failed. It carries no cost: a Call is token-denominated, and money is
// computed on read by joining the row to the ai_model_rate effective on
// its day (ADR-0067 price-on-read — PriceCall and the CostReport SQL),
// so that correcting a rate re-prices history instead of leaving a frozen
// number behind. Nothing on the write path knows a price exists.
// A single logical call — one Complete/CompleteStructured/Embed invocation
// from the caller's point of view — can span several Call rows sharing one
// LogicalCallID when the router retries, degrades, or escalates: every
// rung it actually walked lands its own row, and IsTerminal names the one
// whose response the caller received. It is telemetry, not a domain
// write — no audit/outbox ride-along.
type Call struct {
	// LogicalCallID groups every attempt of one served-or-failed decision.
	// Minted once per logical call (ids.NewV7()) and shared by every Call
	// appended under it.
	LogicalCallID ids.UUID
	// Attempt is this row's 1-based position within its logical call.
	Attempt int
	// IsTerminal marks the attempt whose outcome the caller actually got —
	// the served response or the final failure. Exactly one row per
	// logical call carries it.
	IsTerminal bool
	// AttemptReason names why THIS attempt ran, distinct from the first:
	// "provider_error" (a ladder rung failed and the walk moved to the
	// next), "schema_invalid" (a structured-output retry or escalation),
	// "budget_degrade" (the budget guardrail forced a demoted ladder on
	// what is still attempt 1). Empty for an ordinary first attempt.
	AttemptReason string
	// Kind distinguishes a chat-ladder attempt from an embed-lane call —
	// callKindCompletion or callKindEmbedding.
	Kind          string
	CorrelationID *ids.UUID
	// Subject is the record the call was about, when the site that made it
	// said so. It travels to the rail's occurrence and never to ai_call: the
	// trace is about the call, the occurrence is about the reader's work.
	Subject               Subject
	Task                  Task
	Tier                  Tier
	Provider              string
	ModelID               string
	RequestFingerprint    string
	ContextScopes         []string
	ContextFingerprint    string
	ContextBytes          int
	ContextTokensEstimate int
	TokensIn              int
	TokensOut             int
	ReasoningTokens       int
	CachedTokens          int
	// CacheWriteTokens is the cache-creation bucket a native provider
	// reports (e.g. Anthropic's cache_creation_input_tokens) — disjoint
	// from CachedTokens (a read), already counted inside TokensIn, 0 when
	// the provider reports none. The pricer's fourth bucket (ADR-0067).
	CacheWriteTokens int
	LatencyMS        int64
	CacheHit         bool
	// CacheOff records that the serving Router had the result cache
	// disabled (ai.WithoutResultCache — the cert lane and scripted tests
	// that must observe every repeat call, not a collapsed cache hit).
	CacheOff bool
	// ServedModel is the provider-reported identity of the model that
	// actually answered (model.Response.ServedModel), and ServedIdentitySource
	// names how that identity was obtained: "response" when the adapter reads
	// it off the wire response body, "echo" when the generic OpenAI-compatible
	// wire merely echoes back the requested model rather than confirming what
	// served it, "configured" when the provider reported no identity at all
	// and the trace falls back to the tier's configured binding.
	ServedModel          string
	ServedIdentitySource string
	// ServedProvider is the upstream that generated the completion when a
	// broker sat between us and it (model.Response.ServedProvider), empty on a
	// direct vendor. Persisted beside ServedIdentitySource rather than folded
	// into it: the source says how much to trust ServedModel, and this says who
	// served — a broker answers the second confidently while the first stays
	// "echo", so one field cannot carry both.
	ServedProvider string
	// FinishReason is the provider's normalized stop reason, empty when none was
	// reported. Recorded because a truncated answer and a complete one are
	// otherwise the same row, and the difference decides whether a schema
	// failure was the model's fault or the output budget's.
	FinishReason  string
	Degraded      bool
	ErrorSentinel string
	AgentRunID    *ids.UUID
	// ConfigHash points at the ai_call_config row describing the task
	// contract, routing config, and prompt version that produced this
	// attempt. Nil when the serving Router never installed a config
	// snapshot (a DB-less local router with no CallRecorder wired).
	ConfigHash *string
	// Payload, when non-nil, carries the opt-in post-stripper content
	// (Layer 3). It is written to ai_call_payload in the SAME transaction
	// so the content row can never outlive its metadata row. Only the
	// terminal attempt of a logical call ever carries one — the router
	// strips it from any row a later attempt supersedes before flushing.
	Payload *Payload
}

// Payload is the Layer-3 opt-in content: the post-SecretStripper request
// (system + messages) and the model's response text. Special-category-
// adjacent — retention-aged and erasure-cascaded, never in audit_log.
type Payload struct {
	Request  json.RawMessage
	Response json.RawMessage
}

// ConfigSnapshot is one row of the ai_call_config dimension (spec §4): the
// task contract, routing config, and prompt version combination a batch of
// ai_call rows was produced under. Hash is the sha256 digest of the other
// four fields (computeConfigHash) — the append-only table's primary key,
// so the same combination collapses onto one row across every workspace.
type ConfigSnapshot struct {
	Hash              string
	TaskContractHash  string
	RoutingConfigHash string
	PromptVersion     string
	ProviderParams    json.RawMessage
}

// CallRecorder is what the router needs to trace calls; the interface
// keeps router unit tests off Postgres while CallMeter is the one real
// impl. Exported so the DB-less local router seam (ai.WithCallStore) can
// take a caller-supplied recorder (an in-memory store for a cert run or a
// test) without reaching into Postgres.
type CallRecorder interface {
	// Record writes every attempt of one logical call in ONE transaction —
	// the store must never observe a logical call half-written.
	Record(ctx context.Context, attempts []Call) error
	// EnsureConfig plants snap's row if no row for its Hash exists yet
	// (INSERT … ON CONFLICT DO NOTHING) — idempotent, safe to call once per
	// flush regardless of whether this combination was already seen.
	EnsureConfig(ctx context.Context, snap ConfigSnapshot) error
}

// callStore is the pre-existing internal name, kept as an alias so every
// call site inside this package (Router.calls, NewRouter, assembleRouter)
// compiles unchanged.
type callStore = CallRecorder

// CallMeter writes ai_call (+ ai_call_payload when capture is on). It rides
// the workspace GUC transaction like every tenant write.
type CallMeter struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
	// log carries an announce that could not be made. The trace itself never
	// reports through here — Record's error is the caller's to handle — but the
	// rail announcement is deliberately unable to fail a working call, so the
	// log line is the only place its absence is visible.
	log *slog.Logger
}

// NewCallMeter constructs the CallMeter that writes ai_call trace rows
// (and, when payload capture is on, the linked ai_call_payload row).
func NewCallMeter(db *database.DB) *CallMeter {
	return &CallMeter{db: db, log: slog.Default()}
}

// WithLogger points the meter's announce-failure log at a caller's logger.
func (m *CallMeter) WithLogger(log *slog.Logger) *CallMeter {
	if log != nil {
		m.log = log
	}
	return m
}

// Record writes every attempt's ai_call row — and, for whichever attempt
// carries a Payload (only ever the terminal one), the ai_call_payload
// row — in ONE workspace transaction, so a logical call is never observed
// half-written and a content row can never outlive its metadata row.
func (m *CallMeter) Record(ctx context.Context, attempts []Call) error {
	if len(attempts) == 0 {
		return nil
	}
	return m.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := m.recordAttempts(ctx, tx, attempts); err != nil {
			return err
		}
		// The occurrence rides the same transaction as the rows it describes,
		// so the trace and the rail can never disagree about whether the call
		// happened. Only the terminal attempt is announced: a logical call that
		// walked three rungs is one piece of work, not three.
		for _, c := range attempts {
			if c.IsTerminal {
				m.announceRailBestEffort(ctx, tx, c)
			}
		}
		return nil
	})
}

// aiCallBindings pairs each ai_call column this writer fills with the value
// bound to it, and the statement's column list, its placeholders and its bind
// list are derived from the result — the same shape voice_profile_version's
// writer uses two files over.
//
// Paired rather than written as three parallel lists because nothing checks
// that a statement's columns, placeholders and arguments agree: this table
// takes thirty-one of them, and the failure mode of a miscount is not a
// compile error but a value landing in the neighbouring column.
// The column names here are schema identifiers. Two of them happen to be
// spelled the same as a log key in tracing.go and a request-field name in
// ratewrite.go, which is a coincidence of English rather than a shared concept:
// a constant spanning the three would tie a column's name to a validation
// message, and renaming either would then have to argue with the other.
//
//nolint:goconst // "provider"/"model_id" here are ai_call columns, unrelated to the same words used as a log key and a wire field name elsewhere in this package
func aiCallBindings(c Call) []boundColumn {
	// kind and served_identity_source carry a CHECK constraint, not just a SQL
	// DEFAULT — every column is listed explicitly, so an unset Go zero-value
	// would reach the constraint as '""' rather than falling back to the
	// schema's default. Mirror the schema's own defaults here so a caller that
	// does not care about these fields (most non-router test fixtures) still
	// writes a valid row.
	kind := c.Kind
	if kind == "" {
		kind = callKindCompletion
	}
	servedSource := c.ServedIdentitySource
	if servedSource == "" {
		servedSource = servedIdentitySourceConfigured
	}
	contextScopes := c.ContextScopes
	if contextScopes == nil {
		contextScopes = []string{}
	}
	// error_sentinel is nullable and an absent sentinel is NULL, not ''. The
	// statement used to spell that as NULLIF on the placeholder; with the
	// placeholders derived it is decided here instead, where the rest of this
	// row's optional values are already decided.
	var errorSentinel *string
	if c.ErrorSentinel != "" {
		errorSentinel = &c.ErrorSentinel
	}
	return []boundColumn{
		{"correlation_id", c.CorrelationID},
		{"task", string(c.Task)},
		{"tier", string(c.Tier)},
		{"provider", c.Provider},
		{"model_id", c.ModelID},
		{"request_fingerprint", c.RequestFingerprint},
		{"context_scopes", contextScopes},
		{"context_fingerprint", c.ContextFingerprint},
		{"context_bytes", c.ContextBytes},
		{"context_tokens_estimate", c.ContextTokensEstimate},
		{"tokens_in", c.TokensIn},
		{"tokens_out", c.TokensOut},
		{"reasoning_tokens", c.ReasoningTokens},
		{"cached_tokens", c.CachedTokens},
		{"cache_write_tokens", c.CacheWriteTokens},
		{"latency_ms", c.LatencyMS},
		{"cache_hit", c.CacheHit},
		{"degraded", c.Degraded},
		{"error_sentinel", errorSentinel},
		{"agent_run_id", c.AgentRunID},
		{"logical_call_id", c.LogicalCallID},
		{"attempt", c.Attempt},
		{"is_terminal", c.IsTerminal},
		{"attempt_reason", c.AttemptReason},
		{"kind", kind},
		{"served_model", c.ServedModel},
		{"served_identity_source", servedSource},
		{"served_provider", c.ServedProvider},
		{"finish_reason", c.FinishReason},
		{"cache_off", c.CacheOff},
		{"config_hash", c.ConfigHash},
	}
}

// recordAttempts writes every buffered attempt of one logical call, and the
// ai_call_payload row of whichever attempt carries content.
func (m *CallMeter) recordAttempts(ctx context.Context, tx pgx.Tx, attempts []Call) error {
	for _, c := range attempts {
		bound := aiCallBindings(c)
		args := valuesOf(bound)
		var callID ids.UUID
		err := tx.QueryRow(ctx, storekit.SQLf(
			`INSERT INTO ai_call (%s) VALUES (%s) RETURNING id`,
			strings.Join(namesOf(bound), ", "), bindPlaceholders(len(args))),
			args...).Scan(&callID)
		if err != nil {
			return fmt.Errorf("ai: recording call: %w", err)
		}
		if c.Payload == nil {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO ai_call_payload (ai_call_id, request_payload, response_payload)
			VALUES ($1, $2, $3)`,
			callID, c.Payload.Request, c.Payload.Response)
		if err != nil {
			return fmt.Errorf("ai: recording call payload: %w", err)
		}
	}
	return nil
}

// EnsureConfig plants snap's row in ai_call_config if it does not already
// exist.
func (m *CallMeter) EnsureConfig(ctx context.Context, snap ConfigSnapshot) error {
	// rls-exempt: ai_call_config is a global config-snapshot dimension (spec §4) — no RLS policy, so this write must not ride the per-workspace GUC transaction.
	_, err := m.db.Pool().Exec(ctx, `
		INSERT INTO ai_call_config (hash, task_contract_hash, routing_config_hash, prompt_version, provider_params)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (hash) DO NOTHING`,
		snap.Hash, snap.TaskContractHash, snap.RoutingConfigHash, snap.PromptVersion, []byte(snap.ProviderParams))
	if err != nil {
		return fmt.Errorf("ai: ensuring config snapshot: %w", err)
	}
	return nil
}

// errMeteringFailed marks the terminal where a call was SERVED but the
// usage meter's write failed. It is distinct from a provider error: the
// model answered, only the metering-DB write did not — classifyError maps
// it to its own sentinel so the trace does not mislabel a successful call.
var errMeteringFailed = errors.New("ai: metering failed")

// classifyError maps a completion terminal error to a short, stable code
// for ai_call.error_sentinel. It never stores raw error text — that could
// leak provider internals into the trace store; the code is enough to
// spot patterns, and the failing call's own logs carry the detail.
func classifyError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrBudgetDeferred):
		return "budget_deferred"
	case errors.Is(err, errMeteringFailed):
		return "metering_failed"
	case errors.Is(err, errBudgetUnavailable):
		return "budget_unavailable"
	case errors.Is(err, errRequestFailed):
		return "request_failed"
	// A REFUSAL IS NOT A FAILURE TO ANSWER, and the three are ordered widest
	// last because a volume budget and a throttle both wrap a refusal.
	//
	// An operator reads this trace to decide what to do. "provider_error" says
	// try again or rebind; an exhausted account says top up, and a burst limit
	// says wait. Folding all three into one bucket sends somebody to a console
	// where nothing is wrong — which is the same misdiagnosis the voice screen
	// carried until the two sentinels were split out, and this is the surface
	// used to diagnose model health everywhere else.
	case errors.Is(err, ErrProviderQuota):
		return "provider_quota"
	case errors.Is(err, ErrProviderThrottled):
		return "provider_throttled"
	case errors.Is(err, errProviderRefused):
		return "provider_refused"
	default:
		return "provider_error"
	}
}

// The two non-provider failure classes the trace store distinguishes so an
// operator can tell a flaky vendor from our own outage or a request that
// never reached routing: a budget read that broke, and a caller-side
// preparation that died before any model was tried.
var (
	errBudgetUnavailable = errors.New("ai: the budget could not be read")
	errRequestFailed     = errors.New("ai: the request failed before routing")
)
