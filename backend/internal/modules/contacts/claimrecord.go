// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ourPromiseNotYetATask is an open promise of ours that no task holds yet.
//
// Every reader that counts promises beside the open tasks uses it, because a
// promise the dispatch turned into a task is that task from then on: counting
// the claim as well would show one promise twice. Readers of one record's
// claims keep the task-backed ones, so the quote and the task stay reachable.
const ourPromiseNotYetATask = ourPromiseWithNoTask + ` AND c.status = 'open'`

// ourPromiseWithNoTask is ourPromiseNotYetATask whatever its status: the
// claims a Worklist promise row names, open or settled since.
const ourPromiseWithNoTask = `c.kind = 'commitment_ours' AND NOT c.needs_review AND c.task_activity_id IS NULL`

// RecordConversationClaimTx files an extracted claim inside the caller's
// transaction, so the claim commits with whatever the extraction did about it.
//
// A claim already filed for the same evidence is returned rather than written
// again, with created false. Its status is the memory the caller acts on: a
// claim a human dismissed stays dismissed however often the conversation is
// read.
func (s *Store) RecordConversationClaimTx(
	ctx context.Context, tx pgx.Tx, in ClaimInput,
) (crmcontracts.ConversationClaim, bool, error) {
	if err := validateClaim(in); err != nil {
		return crmcontracts.ConversationClaim{}, false, err
	}
	// RequireHuman refuses an agent passport and admits the product's own
	// passes, which is what an extractor runs as.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.ConversationClaim{}, false, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return crmcontracts.ConversationClaim{}, false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ConversationClaim{}, false, err
	}
	return recordClaimInTx(ctx, tx, in, by)
}

// validateClaim names a malformed claim before any authority question, so a
// caller's own omission is not reported as a permission problem.
func validateClaim(in ClaimInput) error {
	if !values.HasVisibleText(in.Body) {
		return httperr.Validation("body", "required",
			"a claim says something; an empty one is not a claim")
	}
	// An omitted id decodes to the zero UUID with no error, so without this
	// probe it would reach the visibility check and answer not-found for a
	// message nobody named.
	if err := httperr.RequireBodyID("source_activity_id", in.ActivityID); err != nil {
		return err
	}
	if !values.HasVisibleText(in.Quote) {
		return httperr.Validation("source_quote", "required",
			"a claim carries the words it was read from — an ungrounded claim is dropped, never stored")
	}
	return nil
}

// recordClaimInTx is the one insert both writers share.
func recordClaimInTx(
	ctx context.Context, tx pgx.Tx, in ClaimInput, by string,
) (crmcontracts.ConversationClaim, bool, error) {
	// HELD, not merely probed: an Art. 17 erasure committing between a probe
	// and the insert would put the verbatim sentence back against a contact
	// the operator has just been told is gone.
	if err := auth.HoldWritableLive(ctx, tx, "contact", in.ContactID.UUID); err != nil {
		return crmcontracts.ConversationClaim{}, false, err
	}
	// Live, not merely visible: a claim must not quote an archived message.
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, in.ActivityID); err != nil {
		return crmcontracts.ConversationClaim{}, false, err
	}
	if err := requireQuoteInActivity(ctx, tx, in); err != nil {
		return crmcontracts.ConversationClaim{}, false, err
	}
	fingerprint := claimFingerprint(in)
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO conversation_claim
			(contact_id, kind, body, source_activity_id, source_quote,
			 due_at, evidence_fingerprint, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (contact_id, source_activity_id, evidence_fingerprint)
		   WHERE archived_at IS NULL DO NOTHING
		RETURNING id`,
		in.ContactID, in.Kind, in.Body,
		in.ActivityID, in.Quote, in.DueAt,
		fingerprint, in.Source, by).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := filedClaim(ctx, tx, in, fingerprint)
		return existing, false, err
	}
	if err != nil {
		return crmcontracts.ConversationClaim{}, false, fmt.Errorf("write the conversation claim: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "create", "contact", in.ContactID.UUID, nil,
		map[string]any{"claim_kind": in.Kind, claimIDKey: id.String()})
	if err != nil {
		return crmcontracts.ConversationClaim{}, false, fmt.Errorf("audit the conversation claim: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, in.ContactID.UUID,
		crmcontracts.PublicEventConversationClaimCaptured{
			ClaimId: openapi_types.UUID(id),
			Kind:    in.Kind,
		}); err != nil {
		return crmcontracts.ConversationClaim{}, false, fmt.Errorf("emit conversation_claim.captured: %w", err)
	}
	return crmcontracts.ConversationClaim{
		Id:               openapi_types.UUID(id),
		Kind:             crmcontracts.ConversationClaimKind(in.Kind),
		Body:             in.Body,
		SourceActivityId: openapi_types.UUID(in.ActivityID),
		SourceQuote:      in.Quote,
		Status:           crmcontracts.ConversationClaimStatusOpen,
		DueAt:            in.DueAt,
	}, true, nil
}

// requireQuoteInActivity refuses a claim whose quote is not in the message it
// cites. A quote nobody can find in what was written grounds nothing, and it
// would read as trustworthy as one that does.
func requireQuoteInActivity(ctx context.Context, tx pgx.Tx, in ClaimInput) error {
	var subject, body string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(subject, ''), COALESCE(body, '') FROM activity WHERE id = $1`,
		in.ActivityID).Scan(&subject, &body); err != nil {
		return fmt.Errorf("read the message a claim cites: %w", err)
	}
	if values.Quoted(body, in.Quote) || values.Quoted(subject, in.Quote) {
		return nil
	}
	return httperr.Validation("source_quote", "not_in_source",
		"source_quote is not in the message the claim cites; quote its words as written")
}

// filedClaim reads back the claim an earlier reading filed for this evidence.
func filedClaim(
	ctx context.Context, tx pgx.Tx, in ClaimInput, fingerprint string,
) (crmcontracts.ConversationClaim, error) {
	var claim crmcontracts.ConversationClaim
	var id, activityID ids.UUID
	var taskID *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id, kind, body, source_activity_id, source_quote, status, due_at,
		       needs_review, corrected_at, task_activity_id
		  FROM conversation_claim
		 WHERE contact_id = $1 AND source_activity_id = $2 AND evidence_fingerprint = $3
		   AND archived_at IS NULL`,
		in.ContactID, in.ActivityID, fingerprint).Scan(&id, &claim.Kind, &claim.Body,
		&activityID, &claim.SourceQuote, &claim.Status, &claim.DueAt,
		&claim.NeedsReview, &claim.CorrectedAt, &taskID)
	if err != nil {
		return crmcontracts.ConversationClaim{}, fmt.Errorf("read the claim already filed for this evidence: %w", err)
	}
	claim.Id = openapi_types.UUID(id)
	claim.SourceActivityId = openapi_types.UUID(activityID)
	if taskID != nil {
		task := openapi_types.UUID(*taskID)
		claim.TaskActivityId = &task
	}
	return claim, nil
}

// SetClaimTaskTx records the task a commitment became, so a reader holding
// the claim can open the task and an aggregate reader counts the promise once.
// A claim that already names a task keeps it: the first task written for a
// promise is the one its history points at.
func (s *Store) SetClaimTaskTx(ctx context.Context, tx pgx.Tx, claimID, taskID ids.UUID) error {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	if _, err := storekit.LockRow(ctx, tx, "conversation_claim", claimID, storekit.LiveOnly); err != nil {
		return err
	}
	var contactID ids.UUID
	var status string
	err := tx.QueryRow(ctx, `
		UPDATE conversation_claim SET task_activity_id = $2
		 WHERE id = $1 AND task_activity_id IS NULL
		RETURNING contact_id, status`, claimID, taskID).Scan(&contactID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("link the claim to its task: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "contact", contactID,
		map[string]any{claimIDKey: claimID.String(), "task_activity_id": nil},
		map[string]any{claimIDKey: claimID.String(), "task_activity_id": taskID.String()})
	if err != nil {
		return fmt.Errorf("audit the claim's task: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, contactID,
		crmcontracts.PublicEventConversationClaimChanged{ClaimId: openapi_types.UUID(claimID), Status: status}); err != nil {
		return fmt.Errorf("emit conversation_claim.changed: %w", err)
	}
	return nil
}

// ContactNamedAmong finds the one contact among the named ones whose full name
// is this one, compared without case. A meeting names its speakers rather than
// addressing them, so the name is how a promise is tied to the customer who
// made it; two contacts of that name tie it to nobody.
func (s *Store) ContactNamedAmong(
	ctx context.Context, tx pgx.Tx, among []ids.UUID, name string,
) (ids.ContactID, bool, error) {
	if len(among) == 0 || name == "" {
		return ids.ContactID{}, false, nil
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return ids.ContactID{}, false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	amongPos, namePos := arg(among), arg(name)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return ids.ContactID{}, false, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id FROM contact c
		 WHERE c.id = ANY($%d) AND lower(c.full_name) = lower($%d)
		   AND c.archived_at IS NULL AND (%s)
		 LIMIT 2`, amongPos, namePos, scope), args...)
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("find the contact a meeting names: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("find the contact a meeting names: %w", err)
	}
	if len(found) != 1 {
		return ids.ContactID{}, false, nil
	}
	return ids.From[ids.ContactKind](found[0]), true, nil
}
