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
	RequestTypeField *string              `json:"request_type_field,omitempty"`
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
				args := pgx.NamedArgs{argHandler: inquiryRecipeName, "occurrence": key, "fingerprint": inquiryPlanFingerprint}
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

// InquiryPlan reads the plan frozen for a submission key, if there is one.
func (s *EffectClaimStore) InquiryPlan(ctx context.Context, key string) (InquiryPlan, bool, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return InquiryPlan{}, false, err
	}
	plans, err := s.inquiryPlans(ctx, []string{key})
	plan, found := plans[key]
	return plan, found, err
}

func (s *EffectClaimStore) inquiryPlans(ctx context.Context, keys []string) (map[string]InquiryPlan, error) {
	out := map[string]InquiryPlan{}
	if len(keys) == 0 {
		return out, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT occurrence_key,planned FROM automation_effect_claim WHERE handler=@handler AND occurrence_key=ANY(@keys) AND effect_fingerprint=@fingerprint AND planned IS NOT NULL`, pgx.NamedArgs{argHandler: inquiryRecipeName, argKeys: keys, "fingerprint": inquiryPlanFingerprint})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var raw json.RawMessage
			if err := rows.Scan(&key, &raw); err != nil {
				return err
			}
			var plan InquiryPlan
			if err := json.Unmarshal(raw, &plan); err != nil {
				return err
			}
			out[key] = plan
		}
		return rows.Err()
	})
	return out, err
}
