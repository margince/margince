// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package proposalstage asks a deal's owner whether to move the deal to the
// proposal stage after a proposal went out. It also writes the accepted move.
package proposalstage

// A proposal that went out on an open deal asks the deal's owner whether to
// move the deal to the proposal stage. The question is an approval of kind
// deals.ProposalStageKind, and the owner's accept is the only move. The
// offer trigger, the mail reading and the attachment-name check all raise it
// through Suggester, so one writer decides what is already open.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealscout"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	proposalStageActor       = "system:proposal-stage"
	proposalStageSummaryCode = "approval.proposalStageAsked"
	// SentLabel is the criterion the reading is asked beside a stage's
	// own, under deals.ProposalSentCriterionKey.
	SentLabel = "Our side sends the customer a proposal, quote or offer attached to this message"

	proposalStageWithdrawn = "a proposal no longer asks this: the deal moved, closed or the stage changed"
	proposalStageReplaced  = "a newer proposal names another stage"
	proposalFileGoneReason = "The mail's attached proposal is gone, withheld or no longer on this deal, so nothing was moved."
	approvalTargetDeal     = "deal"
	activityKindEmail      = "email"
	directionOutbound      = "outbound"
)

// Source is what raised the question: an offer sent, or a mail of ours.
type Source struct {
	Type    string
	ID      ids.UUID
	Snippet string
	// File is the attachment the mail carried. Zero for a mail whose first
	// live file is looked up when the question is raised. Named says the file
	// name is what marked it a proposal.
	File  ids.UUID
	Named bool
}

const LockSQL = `SELECT pg_advisory_xact_lock(hashtextextended('deal_proposal_stage:' || $1::text, 0))`

// proposalLock serializes everything that raises or retires the questions of
// one deal. A row lock cannot do it while no question exists yet, and two
// sources have two identities, so the identity lock does not meet either.
func proposalLock(ctx context.Context, tx pgx.Tx, dealID ids.DealID) error {
	if _, err := tx.Exec(ctx, LockSQL, dealID); err != nil {
		return fmt.Errorf("compose: locking the proposal questions of a deal: %w", err)
	}
	return nil
}

// Suggester raises and retires the questions.
type Suggester struct {
	db    *database.DB
	inbox *approvals.Service
}

// New builds the suggester over the installation's database and the approvals
// service questions are staged on. Compose passes the service in, because it
// is assembled from effects this package is registered into.
func New(db *database.DB, inbox *approvals.Service) *Suggester {
	return &Suggester{db: db, inbox: inbox}
}

// bind makes ctx the system principal the question is staged as. It binds the
// installation's workspace, which a bus envelope does not carry, and keeps the
// correlation id of the event that raised the question.
func (s *Suggester) bind(ctx context.Context) (context.Context, error) {
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	correlation, traced := principal.CorrelationID(ctx)
	if !traced {
		correlation = ids.NewV7()
	}
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: proposalStageActor})
	return principal.WithCorrelationID(principal.WithWorkspaceID(ctx, workspace.UUID), correlation), nil
}

// Suggest asks the deal's owner whether to move it, unless there is nothing
// to ask. A question already open for the same stage absorbs the new source.
func (s *Suggester) Suggest(ctx context.Context, dealID ids.DealID, src Source) error {
	ctx, err := s.bind(ctx)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error { return s.suggestTx(ctx, tx, dealID, src) })
}

func (s *Suggester) suggestTx(ctx context.Context, tx pgx.Tx, dealID ids.DealID, src Source) error {
	if err := proposalLock(ctx, tx, dealID); err != nil {
		return err
	}
	if src.Type == deals.ProposalSourceActivity && src.File.IsZero() {
		file, err := proposalFirstFile(ctx, tx, src.ID)
		if err != nil || file.IsZero() {
			return err
		}
		src.File = file
	}
	move, ok, err := deals.ProposalStageFor(ctx, tx, dealID)
	if err != nil || !ok {
		return err
	}
	open, err := s.inbox.LockPendingOfKindInTx(ctx, tx, deals.ProposalStageKind, dealID.UUID)
	if err != nil {
		return err
	}
	for _, pending := range open {
		var held deals.ProposalStageMove
		if json.Unmarshal(pending.ProposedChange, &held) == nil && held.ToStageID == move.ToStageID {
			return nil
		}
		if _, err := s.inbox.WithdrawInTx(ctx, tx, pending.ID, proposalStageReplaced); err != nil {
			return err
		}
	}
	move.SourceType, move.SourceID = src.Type, src.ID
	if !src.File.IsZero() {
		move.SourceAttachmentID, move.SourceFileNamed = &src.File, src.Named
	}
	return s.stageTx(ctx, tx, move, src)
}

func (s *Suggester) stageTx(ctx context.Context, tx pgx.Tx, move deals.ProposalStageMove, src Source) error {
	body, err := json.Marshal(move)
	if err != nil {
		return fmt.Errorf("compose: encoding the proposal stage question: %w", err)
	}
	identity, err := json.Marshal(map[string]string{
		"deal_id": move.DealID.String(), "to_stage_id": move.ToStageID.String(),
		"source_type": move.SourceType, "source_id": move.SourceID.String(),
	})
	if err != nil {
		return fmt.Errorf("compose: encoding the proposal stage identity: %w", err)
	}
	digest := sha256.Sum256(body)
	ttl := deals.ProposalStageTTLDays * 24 * time.Hour
	_, _, err = s.inbox.StageUnlessDeclinedTx(ctx, tx, approvals.StageInput{
		Kind:           deals.ProposalStageKind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     approvalTargetDeal,
		TargetID:       move.DealID,
		Summary: fmt.Sprintf(systemtext.Copy(proposalStageSummaryCode, textlang.English),
			move.DealName, move.ToStageName),
		SystemText: &systemtext.Message{Version: 1, Subject: systemtext.Sentence(proposalStageSummaryCode,
			systemtext.Text(move.DealName), systemtext.Text(move.ToStageName))},
		JoinPending: true,
		Identity:    identity,
		TTL:         &ttl,
		Evidence:    []approvals.Evidence{proposalEvidence(move, src)},
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return nil
	}
	return err
}

// proposalEvidence is what the owner judges the question on. It is the line of
// the mail that says a proposal is attached, or the offer that was sent.
func proposalEvidence(move deals.ProposalStageMove, src Source) approvals.Evidence {
	sourceType, sourceID := src.Type, src.ID
	if sourceType == deals.ProposalSourceOffer {
		sourceType, sourceID = approvalTargetDeal, move.DealID
	}
	snippet := textcut.Bytes(src.Snippet, approvals.MaxEvidenceSnippet)
	if snippet == "" {
		snippet = move.DealName
	}
	return approvals.Evidence{Snippet: snippet, SourceType: sourceType, SourceID: sourceID}
}

// Retire withdraws the deal's open questions that have nothing left to ask.
func (s *Suggester) Retire(ctx context.Context, dealID ids.DealID) error {
	ctx, err := s.bind(ctx)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := proposalLock(ctx, tx, dealID); err != nil {
			return err
		}
		open, err := s.inbox.LockPendingOfKindInTx(ctx, tx, deals.ProposalStageKind, dealID.UUID)
		if err != nil {
			return err
		}
		for _, pending := range open {
			var held deals.ProposalStageMove
			outdated := json.Unmarshal(pending.ProposedChange, &held) != nil
			if !outdated {
				if outdated, err = deals.ProposalStageOutdated(ctx, tx, held); err != nil {
					return err
				}
			}
			if !outdated {
				continue
			}
			if _, err := s.inbox.WithdrawInTx(ctx, tx, pending.ID, proposalStageWithdrawn); err != nil {
				return err
			}
		}
		return nil
	})
}

// SuggestForAttachedFile raises the question for a mail of ours whose
// attachment is named like a proposal. It is the arm that needs no model.
func (s *Suggester) SuggestForAttachedFile(ctx context.Context, mail deals.ActivityAuthorship, activityID ids.UUID) error {
	if !MailEligible(mail) {
		return nil
	}
	ctx, err := s.bind(ctx)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		file, name, err := proposalNamedFile(ctx, tx, activityID)
		if err != nil || file.IsZero() {
			return err
		}
		return s.suggestTx(ctx, tx, mail.DealID, Source{
			Type: deals.ProposalSourceActivity, ID: activityID, Snippet: "Attached file: " + name,
			File: file, Named: true,
		})
	})
}

// MailEligible is whether a mail can announce a proposal: ours,
// outbound, an email. Participants and the buyer's own mail never do.
func MailEligible(mail deals.ActivityAuthorship) bool {
	return mail.Kind == activityKindEmail && mail.Direction == directionOutbound && mail.AuthorSide == deals.AuthorSeller
}

// proposalNamedFile answers the first live attachment of a mail the whole
// workspace may read whose file name says it is a proposal. The id is zero
// when there is none.
func proposalNamedFile(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (ids.UUID, string, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	activity := arg(activityID)
	document := dealscout.ProposalDocumentSQL("at.filename", "at.content_type", arg)
	return firstFile(ctx, tx, fmt.Sprintf(`
		SELECT at.id, at.filename FROM attachment at JOIN activity a ON a.id = at.activity_id
		 WHERE at.activity_id = $%d AND at.archived_at IS NULL AND NOT at.bytes_withheld
		   AND %s AND %s
		 ORDER BY at.created_at, at.id LIMIT 1`, activity, dealscout.WorkspaceEvidence("a"), document), args)
}

// proposalFirstFile answers the first live file of a mail the whole workspace
// may read, for a question the reading raised without naming one.
func proposalFirstFile(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (ids.UUID, error) {
	id, _, err := firstFile(ctx, tx, `
		SELECT at.id, at.filename FROM attachment at JOIN activity a ON a.id = at.activity_id
		 WHERE at.activity_id = $1 AND at.archived_at IS NULL AND NOT at.bytes_withheld
		   AND `+dealscout.WorkspaceEvidence("a")+`
		 ORDER BY at.created_at, at.id LIMIT 1`, []any{activityID})
	return id, err
}

func firstFile(ctx context.Context, tx pgx.Tx, query string, args []any) (ids.UUID, string, error) {
	var id ids.UUID
	var name string
	err := tx.QueryRow(ctx, query, args...).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, "", nil
	}
	if err != nil {
		return ids.Nil, "", fmt.Errorf("compose: reading the attachments of a mail that may carry a proposal: %w", err)
	}
	return id, name, nil
}

// proposalFileStillThere answers why the mail's proposal can no longer be
// relied on, or "" while it can. The file must be live and its bytes kept, on
// a mail the decider may read and that still sits on the deal. A file taken
// for a proposal by its name must still be named like one.
func proposalFileStillThere(ctx context.Context, tx pgx.Tx, move deals.ProposalStageMove) (string, error) {
	if move.SourceAttachmentID == nil {
		return "", nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	file, activity, deal := arg(*move.SourceAttachmentID), arg(move.SourceID), arg(move.DealID)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = "true"
	}
	named := "true"
	if move.SourceFileNamed {
		named = dealscout.ProposalDocumentSQL("at.filename", "at.content_type", arg)
	}
	var there bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (SELECT 1 FROM attachment at JOIN activity a ON a.id = at.activity_id
		 WHERE at.id = $%d AND at.activity_id = $%d AND at.archived_at IS NULL AND NOT at.bytes_withheld
		   AND a.archived_at IS NULL AND %s AND %s
		   AND EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'deal' AND l.deal_id = $%d))`,
		file, activity, scope, named, deal), args...).Scan(&there); err != nil {
		return "", fmt.Errorf("compose: rereading the file that announced a proposal: %w", err)
	}
	if there {
		return "", nil
	}
	return proposalFileGoneReason, nil
}

// proposalMailHasAttachment is whether a mail the whole workspace may read
// carries a live file, which is what makes the reading ask about a proposal.
func proposalMailHasAttachment(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (bool, error) {
	var has bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM attachment at JOIN activity a ON a.id = at.activity_id
		                WHERE at.activity_id = $1 AND at.archived_at IS NULL AND NOT at.bytes_withheld
		                  AND `+dealscout.WorkspaceEvidence("a")+`)`, activityID).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("compose: reading whether a mail carries a file: %w", err)
	}
	return has, nil
}

// QuestionFor decides whether the reading of this mail also asks about
// an attached proposal. It does for our own outbound mail with a file, when
// the deal has a stage to move to.
func QuestionFor(ctx context.Context, tx pgx.Tx, mail deals.ActivityAuthorship, activityID ids.UUID) (bool, error) {
	if !MailEligible(mail) {
		return false, nil
	}
	attached, err := proposalMailHasAttachment(ctx, tx, activityID)
	if err != nil || !attached {
		return false, err
	}
	_, ok, err := deals.ProposalStageFor(ctx, tx, mail.DealID)
	return ok, err
}

// AcceptEffect moves the deal as the deciding owner, through the
// stage move every other writer takes, in the transaction that redeems the
// approval.
func AcceptEffect(svc *approvals.Service, store *deals.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var move deals.ProposalStageMove
		if err := json.Unmarshal(proposedChange, &move); err != nil {
			return fmt.Errorf("compose: decoding the proposal stage question: %w", err)
		}
		active, err := store.ActiveDealColumns(ctx)
		if err != nil {
			return err
		}
		reason := ""
		err = svc.RedeemAndApply(ctx, approvalID, deals.ProposalStageKind, diffHash, func(tx pgx.Tx) error {
			var applyErr error
			if reason, applyErr = proposalFileStillThere(ctx, tx, move); applyErr != nil || reason != "" {
				return applyErr
			}
			reason, applyErr = store.ApplyProposalStageTx(ctx, tx, move, approvalID, active)
			return applyErr
		})
		return obsoleteOr(reason, err)
	}
}

// Precheck refuses an edited answer, and says before the click
// when the question no longer applies.
func Precheck(db *database.DB) approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, edited json.RawMessage) error {
		if len(edited) > 0 {
			return &approvals.InvalidEditError{Cause: errors.New("a proposal stage question is accepted or declined as it stands")}
		}
		var move deals.ProposalStageMove
		if err := json.Unmarshal(staged, &move); err != nil {
			return fmt.Errorf("compose: decoding the proposal stage question: %w", err)
		}
		return db.Tx(ctx, func(tx pgx.Tx) error {
			reason, err := deals.ProposalStageStillApplies(ctx, tx, move)
			if err != nil || reason != "" {
				return obsoleteOr(reason, err)
			}
			reason, err = proposalFileStillThere(ctx, tx, move)
			return obsoleteOr(reason, err)
		})
	}
}

// obsoleteOr answers the approvals engine's "no longer applies" for a reason,
// and the error when there is one.
func obsoleteOr(reason string, err error) error {
	if err != nil || reason == "" {
		return err
	}
	return &approvals.ObsoleteError{Reason: reason}
}

// OnOfferSent asks whether the deal moves to the proposal stage. The offer's
// own event payload is the proof a proposal went out. An unreadable payload is
// skipped, because redelivering it would never make it readable.
func (s *Suggester) OnOfferSent(ctx context.Context, offer ids.UUID, eventPayload json.RawMessage) error {
	var payload crmcontracts.PublicEventOfferSent
	if json.Unmarshal(eventPayload, &payload) != nil {
		return nil
	}
	snippet := "Offer sent"
	if payload.Revision != nil {
		snippet = fmt.Sprintf("Offer sent, revision %d", *payload.Revision)
	}
	return s.Suggest(ctx, ids.From[ids.DealKind](ids.UUID(payload.DealId)), Source{
		Type: deals.ProposalSourceOffer, ID: offer, Snippet: snippet,
	})
}

// FromReading raises the question when a mail's reading says a proposal is
// attached. It does nothing unless the reading was asked and answered yes.
// The answer never reaches the evidence ledger: no criterion row carries the
// reserved key.
func (s *Suggester) FromReading(ctx context.Context, dealID ids.DealID, activityID ids.UUID, answeredYes bool, quote string) error {
	if !answeredYes {
		return nil
	}
	return s.Suggest(ctx, dealID, Source{Type: deals.ProposalSourceActivity, ID: activityID, Snippet: quote})
}
