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
type CallReadStore struct{ db *database.DB }

// NewCallReadStore constructs the workspace-bound AI trace read store.
func NewCallReadStore(db *database.DB) *CallReadStore { return &CallReadStore{db: db} }

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
	TokensIn      int64
	TokensOut     int64
	LatencyMS     int64
	OccurredAt    time.Time
}

// CallDetail joins a terminal summary to routing, context, attempts, and
// optional captured payload content.
type CallDetail struct {
	CallSummary
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
	// Tasks is the workspace's COMPLETE terminal-task set (sorted), carried on
	// every page as filter metadata so the trace dropdown offers every task,
	// not just the ones on the current page. Independent of the page's cursor
	// and task filter.
	Tasks []string
}

// The payload existence check keeps list reads independent of captured
// content size. Its alias stays distinct from the detail join alias. The
// decision probe is a correlated aggregate over ai_call_logical_idx, so a page
// costs one statement however many rows it carries.
const callSummaryColumns = `c.id, c.occurred_at, c.task, c.kind, c.tier, c.provider, c.model_id,
	c.served_model, c.attempt, c.tokens_in, c.tokens_out, c.reasoning_tokens,
	c.cached_tokens, c.latency_ms, c.cache_hit, c.degraded, c.error_sentinel,
	EXISTS (SELECT 1 FROM ai_call_payload pp WHERE pp.ai_call_id = c.id),
	(SELECT COALESCE(bool_or(la.kind = 'decision'), false) FROM ai_call la
	 WHERE la.logical_call_id = c.logical_call_id)`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCallSummary(row rowScanner) (CallSummary, error) {
	var summary CallSummary
	err := row.Scan(&summary.ID, &summary.OccurredAt, &summary.Task, &summary.Kind, &summary.Tier,
		&summary.Provider, &summary.ModelID, &summary.ServedModel, &summary.Attempt,
		&summary.TokensIn, &summary.TokensOut, &summary.ReasoningTokens,
		&summary.CachedTokens, &summary.LatencyMS, &summary.CacheHit, &summary.Degraded,
		&summary.ErrorSentinel, &summary.HasPayload, &summary.DecisionAttempted)
	return summary, err
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

// ListCalls returns terminal attempts newest-first. Retry siblings remain
// available through the detail ladder, not as duplicate list entries.
func (s *CallReadStore) ListCalls(
	ctx context.Context,
	cursor *string,
	limit *int,
	task *string,
) (CallPage, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return CallPage{}, err
	}
	n := storekit.ClampLimit(limit)
	where := "c.is_terminal"
	args := []any{}
	addArg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	if task != nil && *task != "" {
		where += fmt.Sprintf(" AND c.task = $%d", addArg(*task))
	}
	if cursor != nil && *cursor != "" {
		decoded, err := storekit.DecodeCursor(*cursor)
		if err != nil {
			return CallPage{}, err
		}
		where += fmt.Sprintf(
			" AND (c.occurred_at, c.id) < ($%d, $%d)",
			addArg(decoded.CreatedAt), addArg(decoded.ID),
		)
	}

	items := make([]CallSummary, 0, n+1)
	tasks := []string{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, storekit.SQLf(
			`SELECT %s FROM ai_call c WHERE %s ORDER BY c.occurred_at DESC, c.id DESC LIMIT %d`,
			callSummaryColumns, where, n+1,
		), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanCallSummary(rows)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// The complete terminal-task set for the filter dropdown, computed in
		// the SAME tx (one round-trip): array_agg over EVERY terminal row, so
		// it is independent of this page's cursor and task filter — the
		// dropdown stays complete no matter what is on screen. is_terminal
		// matches this list's own universe, so no option filters to nothing.
		return tx.QueryRow(
			ctx,
			`SELECT COALESCE(array_agg(DISTINCT task ORDER BY task), '{}') FROM ai_call WHERE is_terminal`,
		).Scan(&tasks)
	})
	if err != nil {
		return CallPage{}, err
	}
	page, err := finishCallPage(items, n)
	if err != nil {
		return CallPage{}, err
	}
	page.Tasks = tasks
	return page, nil
}

func scanCallDetail(row rowScanner) (CallDetail, ids.UUID, error) {
	var detail CallDetail
	var logicalID ids.UUID
	var requestPayload, responsePayload []byte
	// All four arrive NULL together when the join found no configuration.
	var taskContractHash, routingConfigHash, promptVersion *string
	var providerParams []byte
	err := row.Scan(&detail.ID, &detail.OccurredAt, &detail.Task, &detail.Kind, &detail.Tier,
		&detail.Provider, &detail.ModelID, &detail.ServedModel, &detail.Attempt,
		&detail.TokensIn, &detail.TokensOut, &detail.ReasoningTokens,
		&detail.CachedTokens, &detail.LatencyMS, &detail.CacheHit, &detail.Degraded,
		&detail.ErrorSentinel, &detail.HasPayload, &detail.DecisionAttempted, &detail.CorrelationID,
		&detail.AgentRunID, &detail.ServedIdentitySource, &detail.ConfigHash,
		&detail.ContextScopes, &detail.ContextFingerprint, &logicalID,
		&requestPayload, &responsePayload,
		&taskContractHash, &routingConfigHash, &promptVersion, &providerParams)
	if err != nil {
		return CallDetail{}, ids.UUID{}, err
	}
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
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, storekit.SQLf(
			`SELECT %s, c.correlation_id, c.agent_run_id, c.served_identity_source,
				c.config_hash, c.context_scopes, c.context_fingerprint, c.logical_call_id,
				p.request_payload, p.response_payload,
				cfg.task_contract_hash, cfg.routing_config_hash, cfg.prompt_version, cfg.provider_params
			 FROM ai_call c
			 LEFT JOIN ai_call_payload p ON p.ai_call_id = c.id
			 -- The dimension the call's hash names. LEFT because the hash is
			 -- NULLABLE, not because a row might be missing: ai_call_config_fk
			 -- makes a hash that IS set always have one behind it.
			 LEFT JOIN ai_call_config cfg ON cfg.hash = c.config_hash
			 WHERE c.is_terminal AND c.id = $1`, callSummaryColumns,
		), id)
		var logicalID ids.UUID
		var err error
		detail, logicalID, err = scanCallDetail(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT attempt, is_terminal, kind, tier, provider, model_id, attempt_reason,
				error_sentinel, tokens_in, tokens_out, latency_ms, occurred_at
			FROM ai_call WHERE logical_call_id = $1 ORDER BY attempt ASC`, logicalID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var attempt CallAttempt
			if err := rows.Scan(&attempt.Attempt, &attempt.IsTerminal, &attempt.Kind, &attempt.Tier,
				&attempt.Provider, &attempt.ModelID, &attempt.AttemptReason,
				&attempt.ErrorSentinel, &attempt.TokensIn, &attempt.TokensOut,
				&attempt.LatencyMS, &attempt.OccurredAt); err != nil {
				return err
			}
			detail.Attempts = append(detail.Attempts, attempt)
		}
		return rows.Err()
	})
	if err != nil {
		return CallDetail{}, err
	}
	return detail, nil
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
