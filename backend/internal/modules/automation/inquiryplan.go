// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// InquiryPlan carries only references and routing facts, never submitted content.
type InquiryPlan struct {
	OriginalRunID    ids.UUID             `json:"original_run_id"`
	PolicyID         ids.UUID             `json:"policy_id"`
	PolicyVersion    int64                `json:"policy_version"`
	SourceActivityID ids.UUID             `json:"source_activity_id"`
	Target           datasource.EntityRef `json:"target"`
	Recipient        ids.UUID             `json:"recipient"`
	DueAt            time.Time            `json:"due_at"`
	Reason           string               `json:"reason"`
}

const inquiryPlanFingerprint = "inquiry_plan"

// FreezeInquiry commits the routing write and its durable choice together.
func (s *EffectClaimStore) FreezeInquiry(ctx context.Context, key string, choose func(context.Context) (InquiryPlan, error)) (InquiryPlan, error) {
	if err := auth.Require(ctx, "automation", principal.ActionUpdate); err != nil {
		return InquiryPlan{}, err
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return InquiryPlan{}, err
	}
	ctx = principal.WithWorkspaceID(ctx, workspace.UUID)
	var out InquiryPlan
	run := func() error {
		return database.WithWorkspaceSnapshot(ctx, s.db.Pool(), func(bound context.Context) error {
			return s.db.Tx(bound, func(tx pgx.Tx) error {
				if err := storekit.LockWriteIdentity(bound, tx, provenance.WebsiteInquirySource, key); err != nil {
					return err
				}
				args := pgx.NamedArgs{"handler": inquiryRecipeName, "occurrence": key, "fingerprint": inquiryPlanFingerprint}
				var raw json.RawMessage
				err := tx.QueryRow(bound, `SELECT planned FROM automation_effect_claim
     WHERE handler=@handler AND occurrence_key=@occurrence AND effect_fingerprint=@fingerprint
     FOR UPDATE`, args).Scan(&raw)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if len(raw) != 0 {
					return json.Unmarshal(raw, &out)
				}
				out, err = choose(bound)
				if err != nil {
					return err
				}
				if out.PolicyID == ids.Nil || out.PolicyVersion < 1 || out.SourceActivityID == ids.Nil || out.Target.ID == ids.Nil || out.Recipient == ids.Nil {
					return errors.New("automation: inquiry assignment is incomplete")
				}
				raw, err = json.Marshal(out)
				if err != nil {
					return err
				}
				args["planned"] = raw
				_, err = tx.Exec(bound, `INSERT INTO automation_effect_claim (handler,occurrence_key,effect_fingerprint,planned)
     VALUES (@handler,@occurrence,@fingerprint,@planned)
     ON CONFLICT (handler,occurrence_key,effect_fingerprint) DO UPDATE SET planned=excluded.planned
     WHERE automation_effect_claim.planned IS NULL`, args)
				return err
			})
		})
	}
	err = storekit.RetryLockCycles(ctx, run)
	if err != nil {
		return InquiryPlan{}, err
	}
	return out, nil
}

func (s *EffectClaimStore) InquiryPlan(ctx context.Context, key string) (InquiryPlan, bool, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return InquiryPlan{}, false, err
	}
	var raw json.RawMessage
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT planned FROM automation_effect_claim WHERE handler=@handler AND occurrence_key=@key AND effect_fingerprint=@fingerprint AND planned IS NOT NULL`, pgx.NamedArgs{"handler": inquiryRecipeName, "key": key, "fingerprint": inquiryPlanFingerprint}).Scan(&raw)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return InquiryPlan{}, false, nil
	}
	if err != nil {
		return InquiryPlan{}, false, err
	}
	var out InquiryPlan
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, false, err
	}
	return out, true, nil
}
