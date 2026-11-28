// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReadTeamRules answers the saved team rules to a caller holding the read grant.
func (s *AdminStore) ReadTeamRules(ctx context.Context) (crmcontracts.AiTeamRules, error) {
	var out crmcontracts.AiTeamRules
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = teamRulesTx(ctx, tx)
		return err
	})
	return out, err
}

// ReplaceTeamRules saves the text in one transaction with its audit row. A save
// whose editor read an older version is refused, so two admins cannot silently
// overwrite each other.
func (s *AdminStore) ReplaceTeamRules(ctx context.Context, change crmcontracts.AiTeamRulesChange) (crmcontracts.AiTeamRules, error) {
	if err := auth.Require(ctx, teamRulesObject, principal.ActionUpdate); err != nil {
		return crmcontracts.AiTeamRules{}, err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.AiTeamRules{}, err
	}
	var out crmcontracts.AiTeamRules
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, TeamRulesKey); err != nil {
			return err
		}
		current, err := teamRulesTx(ctx, tx)
		if err != nil {
			return err
		}
		if !sameInstant(current.UpdatedAt, change.ExpectedUpdatedAt) {
			return apperrors.ErrVersionSkew
		}
		raw, err := json.Marshal(strings.TrimSpace(change.Text))
		if err != nil {
			return err
		}
		if _, err := s.settings.SetRawTxReceipt(ctx, tx, TeamRulesKey, raw); err != nil {
			return err
		}
		out, err = teamRulesTx(ctx, tx)
		return err
	})
	return out, err
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// teamRulesTx reads the text through its read gate, with when and by whom it was
// last saved. Both come from the newest audit row, the one record of who changed
// what, so this answer and the audit log give the same editor.
func teamRulesTx(ctx context.Context, tx pgx.Tx) (crmcontracts.AiTeamRules, error) {
	if err := auth.Require(ctx, teamRulesObject, principal.ActionRead); err != nil {
		return crmcontracts.AiTeamRules{}, err
	}
	text, err := settings.GetTx(ctx, tx, TeamRules)
	if err != nil {
		return crmcontracts.AiTeamRules{}, err
	}
	out := crmcontracts.AiTeamRules{Text: text}
	var (
		savedAt time.Time
		editor  *string
	)
	installation, err := storekit.Workspace(ctx, tx)
	if err != nil {
		return crmcontracts.AiTeamRules{}, err
	}
	err = tx.QueryRow(ctx, `
		SELECT a.occurred_at, `+auditattribution.ActorNameColumn+`
		FROM audit_log a`+auditattribution.ActorNameJoins+`
		WHERE a.entity_type = $1 AND a.entity_id = $2
		ORDER BY a.id DESC LIMIT 1`, teamRulesObject, installation).Scan(&savedAt, &editor)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return crmcontracts.AiTeamRules{}, err
	}
	out.UpdatedAt = &savedAt
	out.UpdatedBy = editor
	return out, nil
}
