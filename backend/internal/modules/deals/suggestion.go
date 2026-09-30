// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Deal Scout's suggestions: a company with no open deal, and evidence that
// commercial motion is under way. Compose reads the evidence and decides; this
// file is the writer that holds what a suggestion may be, whoever proposes it.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// suggestionEntity names a suggestion in its audit rows and events.
const suggestionEntity = "deal_suggestion"

// The suggestion lifecycle. Only open is ever shown to a reader.
const (
	SuggestionOpen       = "open"
	SuggestionAccepted   = "accepted"
	SuggestionDismissed  = "dismissed"
	SuggestionSuperseded = "superseded"
)

// The kinds of evidence a suggestion cites, one per deal_suggestion_evidence row.
const (
	EvidenceMeeting    = "meeting"
	EvidenceSignal     = "signal"
	EvidenceAttachment = "attachment"
)

// SuggestionEvidence is one cited item. Exactly one of the three ids is set,
// and it matches Kind.
type SuggestionEvidence struct {
	Kind         string
	ActivityID   *ids.UUID
	SignalID     *ids.UUID
	AttachmentID *ids.UUID
	OccurredAt   time.Time
}

// ref is the item's own id, whichever column carries it.
func (e SuggestionEvidence) ref() ids.UUID {
	for _, id := range []*ids.UUID{e.ActivityID, e.SignalID, e.AttachmentID} {
		if id != nil {
			return *id
		}
	}
	return ids.Nil
}

// SuggestionDraft is what the scout proposes for one company.
type SuggestionDraft struct {
	CompanyID ids.UUID
	// NameHint is which evidence leads, as a code: the suggested deal is named
	// after the company and this hint, never after text read out of a message.
	NameHint string
	// AmountMinor and Currency travel together or not at all: an amount is
	// proposed only when a finished document reading stated both.
	AmountMinor *int64
	Currency    *string
	Confidence  float64
	Evidence    []SuggestionEvidence
}

// The name hints, which the deal_suggestion_name_hint_check CHECK repeats.
const (
	HintProposalSent        = "proposal_sent"
	HintOpportunitySignaled = "opportunity_signalled"
	HintMeetingHeld         = "meeting_held"
)

var nameHints = map[string]bool{HintProposalSent: true, HintOpportunitySignaled: true, HintMeetingHeld: true}

// ErrSuggestionDraftInvalid refuses a draft that breaks the suggestion's own
// shape. The scout builds drafts, so this is a defect in the scout, not input.
var ErrSuggestionDraftInvalid = errors.New("deals: the suggestion draft is malformed")

// suggestionFingerprint hashes the company and the evidence ids. The order the
// scout found the evidence in does not matter; the set does.
func suggestionFingerprint(companyID ids.UUID, evidence []SuggestionEvidence) string {
	parts := make([]string, 0, len(evidence))
	for _, e := range evidence {
		parts = append(parts, e.Kind+":"+e.ref().String())
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(companyID.String() + "|" + strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func validDraft(d SuggestionDraft) error {
	if d.CompanyID.IsZero() || !nameHints[d.NameHint] || len(d.Evidence) == 0 {
		return ErrSuggestionDraftInvalid
	}
	if moneyPairError(d.AmountMinor, nil, d.Currency) != nil || d.Confidence < 0 || d.Confidence > 1 {
		return ErrSuggestionDraftInvalid
	}
	for _, e := range d.Evidence {
		if e.ref().IsZero() || e.OccurredAt.IsZero() {
			return ErrSuggestionDraftInvalid
		}
	}
	return nil
}

// SuggestionFloorExpr is the instant a company's evidence must be newer than:
// the latest dismissal of a suggestion about it, and the latest close of a deal
// on it. Evidence from before a dismissal was judged "not a deal" by a human,
// and evidence from before a close was about the deal that closed. It is NULL
// while neither exists. company is the SQL expression naming the company.
//
// The scout filters its evidence by it and the writer below re-checks every
// item against it.
func SuggestionFloorExpr(company string) string {
	return `greatest(
	  (SELECT max(fs.decided_at) FROM deal_suggestion fs
	    WHERE fs.company_id = ` + company + ` AND fs.state = 'dismissed'),
	  (SELECT max(fd.closed_at) FROM deal fd
	    WHERE fd.company_id = ` + company + ` AND fd.status <> 'open' AND fd.archived_at IS NULL))`
}

// SuggestionCompanyFreeClause says the company has no open deal. The scout, the
// writer, the superseding pass and an acceptance all compose it.
// company is the SQL expression naming the company.
func SuggestionCompanyFreeClause(company string) string {
	return `NOT EXISTS (SELECT 1 FROM deal sod
	          WHERE sod.company_id = ` + company + ` AND sod.status = 'open' AND sod.archived_at IS NULL)`
}

// SuggestableCompanyClause is the condition a company must meet to be offered
// a suggestion: live, not the installation's own, with no open deal and no
// open suggestion. The scout reads candidates through it and the writer
// refuses through it. company is the SQL expression naming the company.
func SuggestableCompanyClause(company string) string {
	return `(EXISTS (SELECT 1 FROM company sco
	          WHERE sco.id = ` + company + ` AND sco.archived_at IS NULL AND NOT sco.is_anchor)
	  AND ` + SuggestionCompanyFreeClause(company) + `
	  AND NOT EXISTS (SELECT 1 FROM deal_suggestion sos
	          WHERE sos.company_id = ` + company + ` AND sos.state = 'open'))`
}

// suggestionFloorTx answers the floor for one company, nil when nothing has
// been dismissed or closed.
func suggestionFloorTx(ctx context.Context, tx pgx.Tx, companyID ids.UUID) (*time.Time, error) {
	var floor *time.Time
	if err := tx.QueryRow(ctx, `SELECT `+SuggestionFloorExpr("$1::uuid"), companyID).Scan(&floor); err != nil {
		return nil, fmt.Errorf("deals: reading the suggestion floor: %w", err)
	}
	return floor, nil
}

// RecordSuggestionTx writes one open suggestion with its evidence, and reports
// whether it was written. System-only: a suggestion is the scout's claim, and a
// seat able to plant one could put a card on every colleague's board.
//
// It writes nothing, without error, when the company already has an open deal
// or an open suggestion, when this exact evidence already raised one, or when
// any item is not newer than the company's floor.
func RecordSuggestionTx(ctx context.Context, tx pgx.Tx, d SuggestionDraft) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if err := validDraft(d); err != nil {
		return false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	floor, err := suggestionFloorTx(ctx, tx, d.CompanyID)
	if err != nil {
		return false, err
	}
	through := d.Evidence[0].OccurredAt
	for _, e := range d.Evidence {
		if floor != nil && !e.OccurredAt.After(*floor) {
			return false, nil
		}
		if e.OccurredAt.After(through) {
			through = e.OccurredAt
		}
	}
	pipelineID, stageID, err := BirthStageTx(ctx, tx, nil, nil)
	if err != nil {
		return false, err
	}
	var id ids.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO deal_suggestion (company_id, pipeline_id, proposed_stage_id, name_hint,
		       proposed_amount_minor, currency, confidence, fingerprint, evidence_count,
		       evidence_through, captured_by)
		SELECT $1::uuid, $2::uuid, $3::uuid, $4::text, $5::bigint, $6::text, $7::numeric, $8::text,
		       $9::integer, $10::timestamptz, $11::text
		 WHERE `+SuggestableCompanyClause("$1::uuid")+`
		ON CONFLICT DO NOTHING
		RETURNING id`,
		d.CompanyID, pipelineID, stageID, d.NameHint, d.AmountMinor, d.Currency,
		d.Confidence, suggestionFingerprint(d.CompanyID, d.Evidence), len(d.Evidence), through, by,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("deals: recording a suggestion: %w", err)
	}
	if err := insertSuggestionEvidence(ctx, tx, id, d.Evidence); err != nil {
		return false, err
	}
	auditID, err := storekit.Audit(ctx, tx, "create", suggestionEntity, id, nil,
		map[string]any{filterCompanyID: d.CompanyID, "evidence_count": len(d.Evidence)})
	if err != nil {
		return false, fmt.Errorf("deals: auditing a suggestion: %w", err)
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventDealSuggestionCreated{
		EvidenceCount: len(d.Evidence),
	}); err != nil {
		return false, fmt.Errorf("deals: emitting deal_suggestion.created: %w", err)
	}
	return true, nil
}

func insertSuggestionEvidence(ctx context.Context, tx pgx.Tx, suggestion ids.UUID, evidence []SuggestionEvidence) error {
	for _, e := range evidence {
		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_suggestion_evidence
			       (suggestion_id, kind, activity_id, signal_id, attachment_id, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			suggestion, e.Kind, e.ActivityID, e.SignalID, e.AttachmentID, e.OccurredAt); err != nil {
			return fmt.Errorf("deals: recording suggestion evidence: %w", err)
		}
	}
	return nil
}

// SupersedeStaleSuggestionsTx retires every open suggestion that no longer
// stands, and reports how many. A suggestion stands while its company has no
// open deal and the suggestion visibility clause, read under the system
// principal, still admits it: the company live, and every piece of evidence
// there and live — a message cited by a signal included. A suggestion that
// clause hides from everybody would otherwise block its company for good.
// System-only, for the same reason recording is.
func SupersedeStaleSuggestionsTx(ctx context.Context, tx pgx.Tx) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	var args []any
	stands, err := suggestionVisibleClause(ctx, func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		UPDATE deal_suggestion s SET state = 'superseded', decided_at = now()
		 WHERE s.state = 'open'
		   AND (NOT `+SuggestionCompanyFreeClause("s.company_id")+` OR NOT `+stands+`)
		RETURNING s.id`, args...)
	if err != nil {
		return 0, fmt.Errorf("deals: superseding suggestions: %w", err)
	}
	retired, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("deals: reading superseded suggestions: %w", err)
	}
	for _, id := range retired {
		if err := recordSuperseded(ctx, tx, id); err != nil {
			return 0, err
		}
	}
	return len(retired), nil
}

// columnState is the audited name of the lifecycle column.
const columnState = "state"

// supersedeTx retires one open suggestion the caller holds locked.
func supersedeTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE deal_suggestion SET state = 'superseded', decided_at = now()
		 WHERE id = $1 AND state = 'open'`, id)
	if err != nil {
		return fmt.Errorf("deals: superseding a suggestion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return recordSuperseded(ctx, tx, id)
}

// recordSuperseded writes the audit row and the event for one retirement.
func recordSuperseded(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	auditID, err := storekit.Audit(ctx, tx, "update", suggestionEntity, id,
		map[string]any{columnState: SuggestionOpen},
		map[string]any{columnState: SuggestionSuperseded})
	if err != nil {
		return fmt.Errorf("deals: auditing a superseded suggestion: %w", err)
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventDealSuggestionSuperseded{}); err != nil {
		return fmt.Errorf("deals: emitting deal_suggestion.superseded: %w", err)
	}
	return nil
}
