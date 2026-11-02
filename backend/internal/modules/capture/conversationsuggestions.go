// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	conversationSuggestionLimit     = 25
	conversationSuggestionScanLimit = 200
	conversationSuggestionBatch     = 25
)

// ConversationSuggestion links an unrecorded address to the readable message supplying it.
type ConversationSuggestion struct {
	Address     string
	DisplayName string
	Activity    ids.ActivityID
}

// ConversationSuggestions reports whether older messages remain outside its bounded scan.
type ConversationSuggestions struct {
	Data        []ConversationSuggestion
	ScanLimited bool
}

type suggestionActivity struct {
	ID        ids.ActivityID
	CreatedAt time.Time
	User      ids.UUID
	Provider  string
}

// ConversationSuggestions returns visible candidates for an explicit record choice.
func (s *Sink) ConversationSuggestions(ctx context.Context, anchor datasource.EntityRef) (ConversationSuggestions, error) {
	out := ConversationSuggestions{Data: []ConversationSuggestion{}}
	if anchor.Type != datasource.EntityContact && anchor.Type != datasource.EntityLead {
		return out, apperrors.ErrInvalidArgument
	}
	err := s.db.Bounded(database.CallerPredicateBudget).Tx(ctx, func(tx pgx.Tx) error {
		if err := requireSuggestionAnchor(ctx, anchor.Type); err != nil {
			return err
		}
		if err := auth.EnsureVisibleLive(ctx, tx, string(anchor.Type), anchor.ID); err != nil {
			return err
		}
		if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
			return err
		}
		var err error
		out, err = s.scanConversationSuggestionsTx(ctx, tx, anchor)
		return err
	})
	return out, err
}

func suggestionActivitiesTx(ctx context.Context, tx pgx.Tx, anchor datasource.EntityRef, after *storekit.Cursor) ([]suggestionActivity, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	anchorArg := arg(anchor.ID)
	anchorSQL := fmt.Sprintf(`EXISTS(SELECT 1 FROM activity_link linked WHERE linked.activity_id=a.id AND linked.lead_id=$%d)`, anchorArg)
	if anchor.Type == datasource.EntityContact {
		anchorSQL = fmt.Sprintf(`(EXISTS(SELECT 1 FROM activity_link linked WHERE linked.activity_id=a.id AND linked.contact_id=$%d) OR EXISTS(SELECT 1 FROM activity_participant known WHERE known.activity_id=a.id AND known.contact_id=$%d))`, anchorArg, anchorArg)
	}
	cursorSQL := ""
	if after != nil {
		cursorSQL = fmt.Sprintf(" AND (a.created_at,a.id)<($%d,$%d)", arg(after.CreatedAt), arg(after.ID))
	}
	query := `SELECT a.id,a.created_at,ps.user_id,ps.provider FROM activity a
 JOIN capture_party_source ps ON ps.activity_id=a.id AND ps.user_id::text=` + auth.ConnectorSeatSQL("a") + ` AND ps.provider=split_part(a.captured_by,':',2)
 WHERE a.kind='email' AND a.direction='inbound' AND a.archived_at IS NULL AND NOT ps.admission_blocked
 AND ` + visible + ` AND ` + anchorSQL + cursorSQL + `
 AND EXISTS(SELECT 1 FROM activity_participant copied WHERE copied.activity_id=a.id AND copied.role IN ('to','cc') AND copied.user_id IS NULL AND copied.contact_id IS NULL AND NOT copied.past_party_cap)
 ORDER BY a.created_at DESC,a.id DESC LIMIT ` + fmt.Sprintf("$%d", arg(conversationSuggestionBatch))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[suggestionActivity])
}

func (s *Sink) suggestionsOnActivityTx(ctx context.Context, tx pgx.Tx, activity suggestionActivity) ([]ConversationSuggestion, error) {
	source := partySource{User: activity.User, Provider: activity.Provider}
	rec, err := s.guardPartyActivity(ctx, tx, activity.ID, &source, false)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	excluded, err := excludedForTx(ctx, tx, rec, source.User)
	if err != nil || excluded != "" {
		return nil, err
	}
	args := []any{activity.ID}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT DISTINCT lower(p.address),coalesce(p.display_name,'') FROM activity_participant p JOIN activity a ON a.id=p.activity_id WHERE p.activity_id=$%d AND p.role IN ('to','cc') AND p.user_id IS NULL AND p.contact_id IS NULL AND NOT p.past_party_cap AND nullif(p.address,'') IS NOT NULL AND `+auth.ParticipantStandsSQL("p", "a")+` AND `+auth.ActivityAvailableClause("a")+` ORDER BY 1,2`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	parties, err := pgx.CollectRows(rows, pgx.RowToStructByPos[admissionParty])
	if err != nil {
		return nil, err
	}
	eligible, err := s.suggestiblePartiesTx(ctx, tx, parties)
	if err != nil {
		return nil, err
	}
	out := []ConversationSuggestion{}
	for _, party := range parties {
		if eligible[party.Email] {
			out = append(out, ConversationSuggestion{Address: party.Email, DisplayName: party.Name, Activity: activity.ID})
		}
	}
	return out, nil
}

func requireSuggestionAnchor(ctx context.Context, kind datasource.EntityType) error {
	switch kind {
	case datasource.EntityContact:
		return auth.Require(ctx, "contact", principal.ActionRead)
	case datasource.EntityLead:
		return auth.Require(ctx, "lead", principal.ActionRead)
	default:
		return apperrors.ErrInvalidArgument
	}
}

func (s *Sink) scanConversationSuggestionsTx(ctx context.Context, tx pgx.Tx, anchor datasource.EntityRef) (ConversationSuggestions, error) {
	out := ConversationSuggestions{Data: []ConversationSuggestion{}}
	seen := map[string]bool{}
	var after *storekit.Cursor
	for scanned := 0; scanned < conversationSuggestionScanLimit; {
		page, err := suggestionActivitiesTx(ctx, tx, anchor, after)
		if err != nil {
			return out, err
		}
		for _, activity := range page {
			scanned++
			suggestions, err := s.suggestionsOnActivityTx(ctx, tx, activity)
			if err != nil {
				return out, err
			}
			for _, suggestion := range suggestions {
				if seen[suggestion.Address] {
					continue
				}
				seen[suggestion.Address] = true
				if len(out.Data) == conversationSuggestionLimit {
					out.ScanLimited = true
					return out, nil
				}
				out.Data = append(out.Data, suggestion)
			}
			after = &storekit.Cursor{CreatedAt: activity.CreatedAt, ID: activity.ID.UUID}
		}
		if len(page) < conversationSuggestionBatch {
			return out, nil
		}
		if len(out.Data) == conversationSuggestionLimit {
			out.ScanLimited = true
			return out, nil
		}
	}
	out.ScanLimited = true
	return out, nil
}
