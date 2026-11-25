// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Suggested links: an extension proposes which record a message belongs to, and
// a reader decides. Core never guesses, so the queue holds only what a unit
// supplied. A proposal writes no activity_link; only an accept does, through the
// same relink every other filing uses, in the transaction that closes the
// suggestion.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	suggestionPending  = "pending"
	suggestionAccepted = "accepted"
	suggestionRejected = "rejected"

	// suggestionProposed is the event's word for a new row. A row's own status
	// starts at pending, so the two words differ.
	suggestionProposed = "proposed"

	maxSuggestionReason = 200
	// suggestionQueueSize bounds one read of the queue. suggestionQueueScan is
	// the page size the read walks the pending rows in.
	suggestionQueueSize = 50
	suggestionQueueScan = 200
)

// linkSuggestionTargets are the records a message may be suggested for. They are
// the names the relink and the attach probe already take.
var linkSuggestionTargets = []string{linkEntityContact, linkEntityCompany, linkEntityDeal, linkEntityProject}

// targetNameColumns is the column that names each target kind on its table.
var targetNameColumns = map[string]string{
	linkEntityContact: columnFullName, linkEntityCompany: "display_name",
	linkEntityDeal: columnName, linkEntityProject: columnName,
}

const fieldSuggestionReason = "reason"

// LinkSuggestionOutcome says what a proposal did.
type LinkSuggestionOutcome string

const (
	// SuggestionCreated is a new row waiting for a reader.
	SuggestionCreated LinkSuggestionOutcome = "created"
	// SuggestionExisting is a repeat: the target was proposed before, whatever
	// became of it, so nothing is written.
	SuggestionExisting LinkSuggestionOutcome = "existing"
	// SuggestionExcluded is the message's own author named as the target. Core
	// drops it, so a unit cannot suggest filing a message under its sender.
	SuggestionExcluded LinkSuggestionOutcome = "excluded"
)

// LinkSuggestionInput is one proposal.
type LinkSuggestionInput struct {
	ActivityID ids.ActivityID
	EntityType string
	EntityID   ids.UUID
	Unit       string
	Reason     string
	Score      float64
}

// ProposeLinkSuggestionTx records a proposal on the caller's transaction, so a
// unit's own rows and the proposal commit together. The caller needs the right
// to update the message and to attach to the target. A record the caller cannot
// read answers not found, so a unit learns nothing about it.
func ProposeLinkSuggestionTx(ctx context.Context, tx pgx.Tx, in LinkSuggestionInput) (LinkSuggestionOutcome, error) {
	if err := validateLinkSuggestion(&in); err != nil {
		return "", err
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return "", err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return "", err
	}
	if err := auth.EnsureActivityWritable(ctx, tx, in.ActivityID.UUID); err != nil {
		return "", err
	}
	if err := auth.EnsureAttachTarget(ctx, tx, in.EntityType, in.EntityID); err != nil {
		return "", err
	}
	if in.EntityType == linkEntityContact {
		author, err := isMessageAuthor(ctx, tx, in.ActivityID, in.EntityID)
		if err != nil {
			return "", err
		}
		if author {
			return SuggestionExcluded, nil
		}
	}
	var id ids.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO activity_link_suggestion (activity_id, entity_type, entity_id, unit, reason, score, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (activity_id, entity_type, entity_id) DO NOTHING
		RETURNING id`,
		in.ActivityID, in.EntityType, in.EntityID, in.Unit, in.Reason, in.Score, capturedBy).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return SuggestionExisting, nil
	}
	if err != nil {
		return "", fmt.Errorf("activities: recording a link suggestion: %w", err)
	}
	return SuggestionCreated, recordLinkSuggestion(ctx, tx, in.ActivityID, id, suggestionProposed, in.EntityType, in.EntityID)
}

func validateLinkSuggestion(in *LinkSuggestionInput) error {
	if !slices.Contains(linkSuggestionTargets, in.EntityType) {
		return &values.ParseError{
			Field: fieldEntityType, Code: "unknown_link_target",
			Message: "a message is suggested for a contact, company, deal or project",
		}
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n == 0 || n > maxSuggestionReason {
		return &values.ParseError{
			Field: fieldSuggestionReason, Code: "bad_suggestion_reason",
			Message: fmt.Sprintf("a suggestion gives one reason of at most %d characters", maxSuggestionReason),
		}
	}
	if in.Score < 0 || in.Score > 1 {
		return &values.ParseError{
			Field: "score", Code: "bad_suggestion_score",
			Message: "a suggestion's score is between 0 and 1",
		}
	}
	return nil
}

// isMessageAuthor reports whether the contact sent the message.
func isMessageAuthor(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, contact ids.UUID) (bool, error) {
	var author bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity_participant
		                WHERE activity_id = $1 AND contact_id = $2 AND role = 'from')`,
		activity, contact).Scan(&author)
	if err != nil {
		return false, fmt.Errorf("activities: checking who wrote the message: %w", err)
	}
	return author, nil
}

// recordLinkSuggestion is the write shape's second half for one suggestion
// change. The audit row names the message, which is the record the queue is
// about, and holds no reason text.
func recordLinkSuggestion(
	ctx context.Context, tx pgx.Tx, activity ids.ActivityID, suggestion ids.UUID,
	status, entityType string, entityID ids.UUID,
) error {
	auditID, err := storekit.AuditEvent(ctx, tx, "update", "activity", activity.UUID, map[string]any{
		"link_suggestion": map[string]any{
			"id": suggestion, fieldStatus: status, fieldEntityType: entityType, fieldEntityID: entityID,
		},
	})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, activity.UUID, crmcontracts.PublicEventActivityLinkSuggestionChanged{
		ActivityId:   openapi_types.UUID(activity.UUID),
		SuggestionId: openapi_types.UUID(suggestion),
		Status:       status,
		TargetType:   entityType,
		TargetId:     openapi_types.UUID(entityID),
	})
}

// suggestionColumns is the select list every read of a suggestion shares.
const suggestionColumns = `s.id, s.activity_id, a.subject, s.entity_type, s.entity_id, s.unit, s.reason, s.score::float8, s.status, s.created_at`

func scanSuggestion(row pgx.Row) (crmcontracts.LinkSuggestion, error) {
	var out crmcontracts.LinkSuggestion
	var id, activity, entity ids.UUID
	var entityType, status string
	var score float64
	if err := row.Scan(&id, &activity, &out.ActivitySubject, &entityType, &entity, &out.Unit, &out.Reason,
		&score, &status, &out.CreatedAt); err != nil {
		return out, err
	}
	out.Id, out.ActivityId, out.EntityId = openapi_types.UUID(id), openapi_types.UUID(activity), openapi_types.UUID(entity)
	out.EntityType = crmcontracts.LinkSuggestionEntityType(entityType)
	out.Status = crmcontracts.LinkSuggestionStatus(status)
	out.Score = float32(score)
	return out, nil
}

// ListLinkSuggestions answers, oldest first, the pending suggestions the caller
// may decide (mayDecide), so the queue shows no row whose buttons the server
// refuses. A caller who may decide none, a read-only seat among them, gets an
// empty queue rather than a refusal.
func (s *Store) ListLinkSuggestions(ctx context.Context) ([]crmcontracts.LinkSuggestion, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	out := []crmcontracts.LinkSuggestion{}
	// The HTTP admission refuses a mutation only to a read seat, so only a read
	// seat is told it may decide nothing.
	if actor, ok := principal.Actor(ctx); !ok || actor.SeatType == principal.SeatRead {
		return out, nil
	}
	// A "no" below is the answer for one row, not the call's refusal, so the
	// checks run unrecorded.
	quiet, err := auth.Unrecorded(ctx, "activity", principal.ActionUpdate)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		out, err = decidableSuggestions(ctx, quiet, tx)
		return err
	})
	return out, err
}

// decidableSuggestions pages through the pending suggestions the caller may
// read, oldest first. It stops at a full queue of ones they may decide, or
// when the rows run out. Read scope is wider than write authority. A fixed
// scan could fill with rows the caller may only read and hide the rest.
// quiet is the unrecorded context the per-row checks run under.
func decidableSuggestions(ctx, quiet context.Context, tx pgx.Tx) ([]crmcontracts.LinkSuggestion, error) {
	out := []crmcontracts.LinkSuggestion{}
	var after *crmcontracts.LinkSuggestion
	for {
		page, err := pendingSuggestions(ctx, tx, after)
		if err != nil {
			return nil, err
		}
		writable, err := writableMessages(quiet, tx, page)
		if err != nil {
			return nil, err
		}
		for _, suggestion := range page {
			if len(out) == suggestionQueueSize {
				return out, nil
			}
			if !writable[ids.UUID(suggestion.ActivityId)] {
				continue
			}
			named, ok, err := decidable(ctx, quiet, tx, suggestion)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, named)
			}
		}
		if len(page) < suggestionQueueScan {
			return out, nil
		}
		after = &page[len(page)-1]
	}
}

// writableMessages narrows a page in one statement to the messages the caller
// may change. A long run of rows they may only read then costs one query per
// page. mayDecide still decides each row that is left.
func writableMessages(ctx context.Context, tx pgx.Tx, page []crmcontracts.LinkSuggestion) (map[ids.UUID]bool, error) {
	messages := make([]ids.UUID, len(page))
	for i, suggestion := range page {
		messages[i] = ids.UUID(suggestion.ActivityId)
	}
	return auth.ActivityWritableSubset(ctx, tx, messages)
}

// decidable answers the suggestion with its target's name when the caller may
// decide it. It answers false when mayDecide refuses or the target is gone.
func decidable(ctx, quiet context.Context, tx pgx.Tx, suggestion crmcontracts.LinkSuggestion) (crmcontracts.LinkSuggestion, bool, error) {
	err := mayDecide(quiet, tx, suggestion)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return suggestion, false, nil
	}
	if err != nil {
		return suggestion, false, err
	}
	name, seen, err := visibleTargetName(ctx, tx, string(suggestion.EntityType), ids.UUID(suggestion.EntityId))
	if err != nil || !seen {
		return suggestion, false, err
	}
	suggestion.EntityName = &name
	return suggestion, true, nil
}

// mayDecide is the one bar for accepting or rejecting a suggestion, asked by
// the decision and by the queue read. The caller updates messages, may change
// this one, and may read the target the answer names. The write scope is the
// relink's, so a reject cannot decide a message an accept would refuse. A
// target they cannot read is not found.
func mayDecide(ctx context.Context, tx pgx.Tx, suggestion crmcontracts.LinkSuggestion) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.EnsureActivityWritable(ctx, tx, ids.UUID(suggestion.ActivityId)); err != nil {
		return err
	}
	seen, err := auth.VisibleTo(ctx, tx, string(suggestion.EntityType), ids.UUID(suggestion.EntityId))
	if err != nil {
		return err
	}
	if !seen {
		return apperrors.ErrNotFound
	}
	return nil
}

// pendingSuggestions reads one page of the pending suggestions whose message
// the caller may read, oldest first, after the given row when there is one.
func pendingSuggestions(ctx context.Context, tx pgx.Tx, after *crmcontracts.LinkSuggestion) ([]crmcontracts.LinkSuggestion, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeUnbounded
	}
	keyset := scopeUnbounded
	if after != nil {
		keyset = fmt.Sprintf("(s.created_at, s.id) > ($%d, $%d)", arg(after.CreatedAt), arg(ids.UUID(after.Id)))
	}
	status, limit := arg(suggestionPending), arg(suggestionQueueScan)
	query := storekit.SQLf(`
		SELECT %s FROM activity_link_suggestion s JOIN activity a ON a.id = s.activity_id
		 WHERE s.status = $%d AND a.archived_at IS NULL AND %s AND %s
		 ORDER BY s.created_at, s.id LIMIT $%d`,
		suggestionColumns, status, scope, keyset, limit)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the link suggestions: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (crmcontracts.LinkSuggestion, error) { return scanSuggestion(r) })
}

// visibleTargetName answers a target's name when the caller may read the
// record, and seen=false when they may not or it is gone.
func visibleTargetName(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) (name string, seen bool, err error) {
	if seen, err = auth.VisibleTo(ctx, tx, entityType, id); err != nil || !seen {
		return "", false, err
	}
	column := targetNameColumns[entityType]
	err = tx.QueryRow(ctx, storekit.SQLf(`SELECT %s FROM %s WHERE id = $1 AND archived_at IS NULL`, column, entityType), id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return name, err == nil, err
}

// AcceptLinkSuggestion files the message under the suggested record and closes
// the suggestion in one transaction. Accepting an accepted suggestion is the
// same success; a rejected one is a conflict.
func (s *Store) AcceptLinkSuggestion(ctx context.Context, id ids.UUID) (crmcontracts.LinkSuggestion, error) {
	return s.decideLinkSuggestion(ctx, id, suggestionAccepted)
}

// RejectLinkSuggestion closes the suggestion and attaches nothing.
func (s *Store) RejectLinkSuggestion(ctx context.Context, id ids.UUID) (crmcontracts.LinkSuggestion, error) {
	return s.decideLinkSuggestion(ctx, id, suggestionRejected)
}

func (s *Store) decideLinkSuggestion(ctx context.Context, id ids.UUID, decision string) (crmcontracts.LinkSuggestion, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.LinkSuggestion{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.LinkSuggestion{}, err
	}
	var out crmcontracts.LinkSuggestion
	err = s.tx(ctx, func(tx pgx.Tx) error {
		out, err = scanSuggestion(tx.QueryRow(ctx, storekit.SQLf(`
			SELECT %s FROM activity_link_suggestion s JOIN activity a ON a.id = s.activity_id
			 WHERE s.id = $1 FOR UPDATE OF s`, suggestionColumns), id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		activity := ids.From[ids.ActivityKind](ids.UUID(out.ActivityId))
		if err := mayDecide(ctx, tx, out); err != nil {
			return err
		}
		if string(out.Status) == decision {
			return nil
		}
		if string(out.Status) != suggestionPending {
			return fmt.Errorf("%w: the suggestion was already %s", apperrors.ErrConflict, out.Status)
		}
		if decision == suggestionAccepted {
			if err := s.fileUnderSuggestion(ctx, tx, activity, out); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE activity_link_suggestion SET status = $2, decided_by = $3, decided_at = now() WHERE id = $1`,
			id, decision, capturedBy); err != nil {
			return fmt.Errorf("activities: deciding a link suggestion: %w", err)
		}
		out.Status = crmcontracts.LinkSuggestionStatus(decision)
		return recordLinkSuggestion(ctx, tx, activity, id, decision, string(out.EntityType), ids.UUID(out.EntityId))
	})
	return out, err
}

// fileUnderSuggestion is the relink an accept performs. A deal also answers the
// question a questioned mail raised, as the single relink does.
func (s *Store) fileUnderSuggestion(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, suggestion crmcontracts.LinkSuggestion) error {
	wrote, err := RelinkActivityInTx(ctx, tx, activity, RelinkActivityInput{
		EntityType: string(suggestion.EntityType), EntityID: ids.UUID(suggestion.EntityId),
	})
	if err != nil {
		return err
	}
	if wrote && suggestion.EntityType == linkEntityDeal {
		return s.answerWhichDealTx(ctx, tx, activity, ids.UUID(suggestion.EntityId))
	}
	return nil
}
