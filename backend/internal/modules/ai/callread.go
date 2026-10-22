// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CallReadStore serves the admin-only AI trace without loading payload
// content into list responses.
type CallReadStore struct {
	db  *database.DB
	now func() time.Time
}

// NewCallReadStore constructs the workspace-bound AI trace read store.
func NewCallReadStore(db *database.DB) *CallReadStore { return &CallReadStore{db: db, now: time.Now} }

// CallSummary is one terminal attempt in the newest-first trace list.
type CallSummary struct {
	ID              ids.UUID
	OccurredAt      time.Time
	Task            string
	Kind            string
	Tier            string
	Provider        string
	ModelID         string
	ServedModel     string
	Attempt         int
	TokensIn        int64
	TokensOut       int64
	ReasoningTokens int64
	CachedTokens    int64
	LatencyMS       int64
	CacheHit        bool
	Degraded        bool
	ErrorSentinel   *string
	HasPayload      bool
	// DecisionAttempted is read across the whole logical call because a
	// fallback's terminal row is the completion, not the decision it replaced.
	DecisionAttempted bool
	// Outcome is the reader's bucket, computed by callOutcomeSQL so the list
	// filter and the label on each row are one definition.
	Outcome string
	// CostMicroUSD prices every attempt of the logical call; nil when an
	// attempt spent tokens with no rate, so a partial sum never reads as whole.
	CostMicroUSD *int64
}

// CallAttempt is one rung in a logical call's oldest-first attempt ladder.
// Kind, tier and binding are carried per attempt because one logical call may
// ask a decision model and then a chat tier, and a reader must see which
// answered and which fell back.
type CallAttempt struct {
	Attempt       int
	IsTerminal    bool
	Kind          string
	Tier          string
	Provider      string
	ModelID       string
	AttemptReason string
	ErrorSentinel *string
	// ServedModel and ServedProvider are what the provider reported serving
	// THIS attempt; "" when it reported nothing.
	ServedModel    string
	ServedProvider string
	// DecisionAnswer is a decision attempt's answer, stood or not; nil on
	// every other kind and on a decision attempt that got no answer.
	DecisionAnswer *DecisionAnswer
	TokensIn       int64
	TokensOut      int64
	LatencyMS      int64
	OccurredAt     time.Time
}

// CallDetail joins a terminal summary to routing, context, attempts, and
// optional captured payload content.
type CallDetail struct {
	CallSummary
	// LogicalCallID is what every attempt of this call shares.
	LogicalCallID        ids.UUID
	CorrelationID        *ids.UUID
	AgentRunID           *ids.UUID
	ServedIdentitySource string
	ConfigHash           *string
	// Config is what ConfigHash resolves to, nil for a call whose hash is
	// absent or whose configuration row was never planted.
	Config             *crmcontracts.AiCallConfig
	ContextScopes      []string
	ContextFingerprint string
	Attempts           []CallAttempt
	Payload            *Payload
}

// CallPage is one keyset-paginated window of terminal call summaries.
type CallPage struct {
	Items      []CallSummary
	NextCursor string
	HasMore    bool
	// Tasks and Models are the workspace's complete terminal sets (sorted).
	// Every page carries them so the dropdowns offer every value, not just
	// the current page's. They ignore cursor and filters.
	Tasks  []string
	Models []string
}

// callSummaryFrom joins each terminal row `c` to two aggregates over its
// logical call's attempts on ai_call_logical_idx. `lc` says whether a decision
// model was asked. `lp` is the attempts' cost, priced as CostReport prices them.
// They are apart so an outcome filter reads only `lc`. That prices just the
// rows it keeps.
func callSummaryFrom(query *callQuery) string {
	fallbackFrom, fallbackTo := rateFallbacks()
	rate := rateMatch("ac.provider", "ac.model_id", "ac.occurred_at::date",
		query.arg(fallbackFrom), query.arg(fallbackTo))
	return storekit.SQLf(`ai_call c
	CROSS JOIN LATERAL (
	SELECT COALESCE(bool_or(la.kind = 'decision'), false) AS decision_attempted
	FROM ai_call la WHERE la.logical_call_id = c.logical_call_id) lc
	CROSS JOIN LATERAL (
	SELECT COALESCE(SUM(%s), 0) AS cost_microusd,
	       COUNT(*) FILTER (WHERE %s) AS unpriced_attempts
	FROM ai_call ac LEFT JOIN LATERAL (%s) r ON true
	WHERE ac.logical_call_id = c.logical_call_id) lp`,
		callCostMicroUSD("ac"), callUnpriced("ac"), rate)
}

// callSummarySelect is the summary column list. The payload probe is an
// existence check so a list read never loads captured content.
func callSummarySelect(outcome string) string {
	return storekit.SQLf(`c.id, c.occurred_at, c.task, c.kind, c.tier, c.provider, c.model_id,
	c.served_model, c.attempt, c.tokens_in, c.tokens_out, c.reasoning_tokens,
	c.cached_tokens, c.latency_ms, c.cache_hit, c.degraded, c.error_sentinel,
	EXISTS (SELECT 1 FROM ai_call_payload pp WHERE pp.ai_call_id = c.id),
	lc.decision_attempted, %s, lp.cost_microusd, lp.unpriced_attempts`, outcome)
}

type rowScanner interface {
	Scan(dest ...any) error
}

// callSummaryScan is the scan target for callSummarySelect's columns, in order;
// finish settles the cost once the row is read.
type callSummaryScan struct {
	summary  *CallSummary
	cost     int64
	unpriced int64
}

func (target *callSummaryScan) dest() []any {
	summary := target.summary
	return []any{
		&summary.ID, &summary.OccurredAt, &summary.Task, &summary.Kind, &summary.Tier,
		&summary.Provider, &summary.ModelID, &summary.ServedModel, &summary.Attempt,
		&summary.TokensIn, &summary.TokensOut, &summary.ReasoningTokens,
		&summary.CachedTokens, &summary.LatencyMS, &summary.CacheHit, &summary.Degraded,
		&summary.ErrorSentinel, &summary.HasPayload, &summary.DecisionAttempted,
		&summary.Outcome, &target.cost, &target.unpriced,
	}
}

func (target *callSummaryScan) finish() {
	if target.unpriced == 0 {
		cost := target.cost
		target.summary.CostMicroUSD = &cost
	}
}

func scanCallSummary(row rowScanner) (CallSummary, error) {
	var summary CallSummary
	target := callSummaryScan{summary: &summary}
	if err := row.Scan(target.dest()...); err != nil {
		return CallSummary{}, err
	}
	target.finish()
	return summary, nil
}

func finishCallPage(items []CallSummary, limit int) (CallPage, error) {
	page := CallPage{Items: items}
	if len(page.Items) <= limit {
		return page, nil
	}
	page.Items = page.Items[:limit]
	last := page.Items[len(page.Items)-1]
	next, err := storekit.EncodeCursor(last.OccurredAt, last.ID)
	if err != nil {
		return CallPage{}, err
	}
	page.NextCursor = next
	page.HasMore = true
	return page, nil
}

// ListCalls returns terminal attempts newest-first, narrowed by filter.
// Retry siblings remain available through the detail ladder, not as
// duplicate list entries.
func (s *CallReadStore) ListCalls(
	ctx context.Context,
	cursor *string,
	limit *int,
	filter CallListFilter,
) (CallPage, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return CallPage{}, err
	}
	n := storekit.ClampLimit(limit)
	query := newCallQuery()
	from := callSummaryFrom(query)
	where := filter.where(query)
	if cursor != nil && *cursor != "" {
		decoded, err := storekit.DecodeCursor(*cursor)
		if err != nil {
			return CallPage{}, err
		}
		where += storekit.SQLf(
			" AND (c.occurred_at, c.id) < ($%d, $%d)",
			query.arg(decoded.CreatedAt), query.arg(decoded.ID),
		)
	}

	var items []CallSummary
	var tasks, models []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, storekit.SQLf(
			`SELECT %s FROM %s WHERE %s ORDER BY c.occurred_at DESC, c.id DESC LIMIT %d`,
			callSummarySelect(query.outcome), from, where, n+1,
		), query.args...)
		if err != nil {
			return err
		}
		items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CallSummary, error) {
			return scanCallSummary(row)
		})
		if err != nil {
			return err
		}
		tasks, models, err = readCallFacets(ctx, tx)
		return err
	})
	if err != nil {
		return CallPage{}, err
	}
	page, err := finishCallPage(items, n)
	if err != nil {
		return CallPage{}, err
	}
	page.Tasks, page.Models = tasks, models
	return page, nil
}

// readCallFacets is the complete terminal task and model sets for the filter
// dropdowns, over every terminal row so they stay complete whatever the page
// shows. is_terminal matches the list's own universe, so no option filters to
// nothing; a call naming no model at all offers no blank option.
func readCallFacets(ctx context.Context, tx pgx.Tx) (tasks, models []string, err error) {
	err = tx.QueryRow(ctx, storekit.SQLf(`SELECT
		COALESCE(array_agg(DISTINCT c.task ORDER BY c.task), '{}'),
		COALESCE(array_agg(DISTINCT %[1]s ORDER BY %[1]s) FILTER (WHERE %[1]s <> ''), '{}')
		FROM ai_call c WHERE c.is_terminal`, callModelSQL),
	).Scan(&tasks, &models)
	return tasks, models, err
}

func scanCallDetail(row rowScanner) (CallDetail, ids.UUID, error) {
	var detail CallDetail
	var logicalID ids.UUID
	var requestPayload, responsePayload []byte
	// All four arrive NULL together when the join found no configuration.
	var taskContractHash, routingConfigHash, promptVersion *string
	var providerParams []byte
	target := callSummaryScan{summary: &detail.CallSummary}
	err := row.Scan(append(target.dest(), &detail.CorrelationID,
		&detail.AgentRunID, &detail.ServedIdentitySource, &detail.ConfigHash,
		&detail.ContextScopes, &detail.ContextFingerprint, &logicalID,
		&requestPayload, &responsePayload,
		&taskContractHash, &routingConfigHash, &promptVersion, &providerParams)...)
	if err != nil {
		return CallDetail{}, ids.UUID{}, err
	}
	target.finish()
	detail.LogicalCallID = logicalID
	if requestPayload != nil && responsePayload != nil {
		detail.Payload = &Payload{Request: requestPayload, Response: responsePayload}
	}
	// The three hashes are NOT NULL on the dimension row, so all of them
	// arriving nil means the call named no configuration — the only way the
	// join misses, since ai_call_config_fk binds any hash that IS set to a row.
	if taskContractHash != nil && routingConfigHash != nil && promptVersion != nil {
		config, err := resolvedConfig(*taskContractHash, *routingConfigHash, *promptVersion, providerParams)
		if err != nil {
			return CallDetail{}, ids.UUID{}, err
		}
		detail.Config = config
	}
	return detail, logicalID, nil
}

// GetCall returns a terminal call and its complete attempt ladder. RLS
// makes a missing and a foreign-workspace identifier indistinguishable.
func (s *CallReadStore) GetCall(ctx context.Context, id ids.UUID) (CallDetail, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return CallDetail{}, err
	}
	var detail CallDetail
	query := newCallQuery()
	from := callSummaryFrom(query)
	idArg := query.arg(id)
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, storekit.SQLf(
			`SELECT %s, c.correlation_id, c.agent_run_id, c.served_identity_source,
				c.config_hash, c.context_scopes, c.context_fingerprint, c.logical_call_id,
				p.request_payload, p.response_payload,
				cfg.task_contract_hash, cfg.routing_config_hash, cfg.prompt_version, cfg.provider_params
			 FROM %s
			 LEFT JOIN ai_call_payload p ON p.ai_call_id = c.id
			 -- The dimension the call's hash names. LEFT because the hash is
			 -- NULLABLE, not because a row might be missing: ai_call_config_fk
			 -- makes a hash that IS set always have one behind it.
			 LEFT JOIN ai_call_config cfg ON cfg.hash = c.config_hash
			 WHERE c.is_terminal AND c.id = $%d`, callSummarySelect(query.outcome), from, idArg,
		), query.args...)
		var logicalID ids.UUID
		var err error
		detail, logicalID, err = scanCallDetail(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		detail.Attempts, err = readCallAttempts(ctx, tx, logicalID)
		return err
	})
	if err != nil {
		return CallDetail{}, err
	}
	return detail, nil
}

// readCallAttempts is one logical call's attempt ladder, oldest first.
func readCallAttempts(ctx context.Context, tx pgx.Tx, logicalID ids.UUID) ([]CallAttempt, error) {
	rows, err := tx.Query(ctx, `
		SELECT attempt, is_terminal, kind, tier, provider, model_id, attempt_reason,
			error_sentinel, served_model, served_provider, decision_choice, decision_confidence,
			tokens_in, tokens_out, latency_ms, occurred_at
		FROM ai_call WHERE logical_call_id = $1 ORDER BY attempt ASC`, logicalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var attempts []CallAttempt
	for rows.Next() {
		var attempt CallAttempt
		var choice *string
		var confidence *float64
		if err := rows.Scan(&attempt.Attempt, &attempt.IsTerminal, &attempt.Kind, &attempt.Tier,
			&attempt.Provider, &attempt.ModelID, &attempt.AttemptReason,
			&attempt.ErrorSentinel, &attempt.ServedModel, &attempt.ServedProvider, &choice, &confidence,
			&attempt.TokensIn, &attempt.TokensOut, &attempt.LatencyMS, &attempt.OccurredAt); err != nil {
			return nil, err
		}
		// The schema holds the two columns NULL together.
		if choice != nil && confidence != nil {
			attempt.DecisionAnswer = &DecisionAnswer{Choice: *choice, Confidence: *confidence}
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

// resolvedConfig assembles what a call's config_hash points at. The caller has
// already established there is one; an object of empty strings would read as a
// configuration that pinned nothing.
func resolvedConfig(taskContract, routing, prompt string, params []byte) (*crmcontracts.AiCallConfig, error) {
	config := &crmcontracts.AiCallConfig{
		TaskContractHash:  taskContract,
		RoutingConfigHash: routing,
		PromptVersion:     prompt,
	}
	if len(params) > 0 {
		var decoded map[string]any
		if err := json.Unmarshal(params, &decoded); err != nil {
			return nil, fmt.Errorf("ai call config provider_params is not an object: %w", err)
		}
		// `jsonb NOT NULL` admits the JSON value `null`, which unmarshals into a
		// nil map without error. Assigning it would hand a reader a non-nil
		// pointer to nothing — a configuration that pinned an empty object,
		// rather than one that pinned none.
		if decoded != nil {
			config.ProviderParams = &decoded
		}
	}
	return config, nil
}
