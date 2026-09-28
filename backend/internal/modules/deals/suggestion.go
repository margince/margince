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
	Name      string
	// AmountMinor and Currency travel together or not at all: an amount is
	// proposed only when a finished document reading stated both.
	AmountMinor *int64
	Currency    *string
	Confidence  float64
	Evidence    []SuggestionEvidence
}

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
	if d.CompanyID.IsZero() || strings.TrimSpace(d.Name) == "" || len(d.Evidence) == 0 {
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

// SuggestableCompanyClause is the condition a company must meet to be offered
// a suggestion: live, not the installation's own, with no open deal and no
// open suggestion. The scout reads candidates through it and the writer
// refuses through it. company is the SQL expression naming the company.
func SuggestableCompanyClause(company string) string {
	return `(EXISTS (SELECT 1 FROM company sco
	          WHERE sco.id = ` + company + ` AND sco.archived_at IS NULL AND NOT sco.is_anchor)
	  AND NOT EXISTS (SELECT 1 FROM deal sod
	          WHERE sod.company_id = ` + company + ` AND sod.status = 'open' AND sod.archived_at IS NULL)
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
		INSERT INTO deal_suggestion (company_id, pipeline_id, proposed_stage_id, proposed_name,
		       proposed_amount_minor, currency, confidence, fingerprint, evidence_count,
		       evidence_through, captured_by)
		SELECT $1::uuid, $2::uuid, $3::uuid, $4::text, $5::bigint, $6::text, $7::numeric, $8::text,
		       $9::integer, $10::timestamptz, $11::text
		 WHERE `+SuggestableCompanyClause("$1::uuid")+`
		ON CONFLICT DO NOTHING
		RETURNING id`,
		d.CompanyID, pipelineID, stageID, strings.TrimSpace(d.Name), d.AmountMinor, d.Currency,
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
// stands, and reports how many. A suggestion stands while its company is live
// with no open deal and every piece of its evidence is still there and still
// live: an archived or erased message behind it retires it. System-only,
// for the same reason recording is.
func SupersedeStaleSuggestionsTx(ctx context.Context, tx pgx.Tx) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		UPDATE deal_suggestion s SET state = 'superseded', decided_at = now()
		 WHERE s.state = 'open'
		   AND (EXISTS (SELECT 1 FROM deal d
		                 WHERE d.company_id = s.company_id AND d.status = 'open' AND d.archived_at IS NULL)
		     OR NOT EXISTS (SELECT 1 FROM company c WHERE c.id = s.company_id AND c.archived_at IS NULL)
		     OR (SELECT count(*) FROM deal_suggestion_evidence e WHERE e.suggestion_id = s.id) <> s.evidence_count
		     OR EXISTS (SELECT 1 FROM deal_suggestion_evidence e
		           LEFT JOIN activity ea ON ea.id = e.activity_id
		           LEFT JOIN signal es ON es.id = e.signal_id
		           LEFT JOIN attachment eat ON eat.id = e.attachment_id
		           LEFT JOIN activity aa ON aa.id = eat.activity_id
		          WHERE e.suggestion_id = s.id AND NOT (
		               (e.kind = 'meeting' AND ea.id IS NOT NULL
		                 AND ea.archived_at IS NULL AND ea.restricted_at IS NULL)
		            OR (e.kind = 'signal' AND es.id IS NOT NULL AND es.archived_at IS NULL)
		            OR (e.kind = 'attachment' AND eat.id IS NOT NULL AND eat.archived_at IS NULL
		                 AND aa.id IS NOT NULL AND aa.archived_at IS NULL AND aa.restricted_at IS NULL))))
		RETURNING s.id`)
	if err != nil {
		return 0, fmt.Errorf("deals: superseding suggestions: %w", err)
	}
	retired, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("deals: reading superseded suggestions: %w", err)
	}
	for _, id := range retired {
		auditID, err := storekit.Audit(ctx, tx, "update", suggestionEntity, id,
			map[string]any{columnState: SuggestionOpen},
			map[string]any{columnState: SuggestionSuperseded})
		if err != nil {
			return 0, fmt.Errorf("deals: auditing a superseded suggestion: %w", err)
		}
		if err := storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventDealSuggestionSuperseded{}); err != nil {
			return 0, fmt.Errorf("deals: emitting deal_suggestion.superseded: %w", err)
		}
	}
	return len(retired), nil
}

// columnState is the audited name of the lifecycle column.
const columnState = "state"
