// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// What the account's open signals say, read once for the two surfaces that
// ask: the strip states the worst, and the contradiction rule asks whether the
// contract ended. Two reads would let them describe two different instants of
// the same account.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/lifecyclereview"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// signalFacts is what the open signals say about the account. Readable is
// false when the caller may not read signals at all: nothing standing and no
// permission to see it are different answers.
type signalFacts struct {
	Readable      bool
	ContractEnded bool
	// Source words and occurrence time support the conflict. Legacy signals
	// keep their reviewable summary without claiming a source date.
	ContractEndedSaid   string
	ContractEndedAt     time.Time
	ContractEndedSource ids.UUID
	// Worst is the most serious signal standing open, or absent when nothing
	// is. Severity first, then the newest of that severity — an older warning
	// that has been sitting there is less news than the one that just arrived.
	Worst    signalHeadline
	HasWorst bool
}

// signalHeadline is one signal as the strip states it.
type signalHeadline struct {
	Kind          string
	Severity      string
	Summary       string
	SummaryParams json.RawMessage
}

// readSignalFacts reads what the account's open signals say: the worst one
// standing, and whether the contract ended. A caller who may not read signals
// gets Readable false rather than an account with nothing standing.
func readSignalFacts(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time) (signalFacts, error) {
	if !auth.ReadGranted(ctx, "signal") {
		return signalFacts{}, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	// The SUBJECT row scope, not the account's. A signal's subject can be a
	// contact or a deal the caller may not see, resolved onto an account they
	// can — and every figure below is drawn from the signal's own text. The
	// canonical company predicate comes from the signals module for the same
	// reason: a second spelling of "belongs to this account" would decide
	// visibility differently from the list this page links to.
	scope, err := auth.SignalScopeClause(ctx, "s", arg)
	if err != nil {
		return signalFacts{}, err
	}
	if scope == "" {
		scope = scopeAll
	}
	facts := signalFacts{Readable: true}
	var kind, severity, summary, endedSaid *string
	var params json.RawMessage
	var endedID *ids.UUID
	// One read serves two readers: the strip states the worst, and the
	// contradiction rule asks whether the contract ended — and what the mail
	// that says so said. Two queries would let them describe two instants.
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		WITH open_signals AS (
			SELECT s.id, s.kind, s.severity, s.summary, s.summary_params, s.detected_at FROM signal s
			 WHERE %[1]s AND s.status = 'open' AND s.archived_at IS NULL AND %[2]s
		)
		SELECT ended.summary, ended.id,
		       worst.kind, worst.severity, worst.summary, worst.summary_params
		  FROM (SELECT 1) one
		  LEFT JOIN LATERAL (
			SELECT id, summary, detected_at FROM open_signals
			 WHERE kind = 'contract_ended'
			 ORDER BY detected_at DESC, id DESC
			 LIMIT 1) ended ON true
		  LEFT JOIN LATERAL (
			SELECT kind, severity, summary, summary_params FROM open_signals
			 ORDER BY CASE severity WHEN 'urgent' THEN 0 WHEN 'warn' THEN 1 ELSE 2 END,
			          detected_at DESC, id DESC
			 LIMIT 1) worst ON true`,
		signals.OfCompanyWhere(companyPos), scope), args...).
		Scan(&endedSaid, &endedID,
			&kind, &severity, &summary, &params); err != nil {
		return signalFacts{}, fmt.Errorf("read the account's open signals: %w", err)
	}
	if endedID != nil {
		finding, err := lifecyclereview.Reconcile(ctx, tx, companyID, now)
		if err != nil {
			return signalFacts{}, err
		}
		facts.ContractEnded = lifecyclereview.Actionable(finding)
		if endedSaid != nil {
			facts.ContractEndedSaid = *endedSaid
		}
		if !finding.SourceID.IsZero() {
			facts.ContractEndedSaid = approvals.SanitizeSummary(finding.Quote)
			facts.ContractEndedAt = finding.OccurredAt
			facts.ContractEndedSource = finding.SourceID
		}
	}
	if kind != nil && severity != nil && summary != nil {
		facts.Worst = signalHeadline{Kind: *kind, Severity: *severity, Summary: *summary, SummaryParams: params}
		facts.HasWorst = true
	}
	return facts, nil
}
