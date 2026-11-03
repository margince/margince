// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	object     = "sales_initiative"
	table      = "sales_initiative"
	recordLead = "lead"
	recordDeal = "deal"
	// A rule read answers at most this many records, the predicate engine's
	// own row bound. Past it a membership is an export and not a list.
	ruleReadLimit = storekit.PredicateRowLimit
)

// RuleMatcher evaluates a rule in a record type's list vocabulary under the
// caller's row scope. The composition root binds the list filter engine.
type RuleMatcher interface {
	Match(ctx context.Context, tx pgx.Tx, recordType string, rule storekit.Predicate, limit int) (matched []ids.UUID, count int, err error)
}

// Store owns the initiative tables.
type Store struct {
	db    *database.DB
	now   func() time.Time
	rules RuleMatcher
}

// NewStore binds the store to db and the rule matcher.
func NewStore(db *database.DB, now func() time.Time, rules RuleMatcher) *Store {
	return &Store{db: db, now: now, rules: rules}
}

// Definition is what a caller sets on an initiative.
type Definition struct {
	Name          string
	Description   *string
	OwnerID       *ids.UUID
	Objective     string
	Target        *int64
	Deadline      time.Time
	LeadRule      *storekit.Predicate
	DealRule      *storekit.Predicate
	IncludeFuture *bool
	Reason        *string
}

// Create writes a draft. Nothing is enrolled until Launch.
func (s *Store) Create(ctx context.Context, in Definition) (crmcontracts.SalesInitiative, error) {
	if err := auth.Require(ctx, object, principal.ActionCreate); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return crmcontracts.SalesInitiative{}, apperrors.ErrPermissionDenied
	}
	if err := checkDefinition(in); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	owner := actor.UserID
	if in.OwnerID != nil {
		owner = *in.OwnerID
	}
	include := in.IncludeFuture == nil || *in.IncludeFuture
	var out crmcontracts.SalesInitiative
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		id := ids.NewV7()
		leadRule, dealRule, err := ruleJSON(in)
		if err != nil {
			return err
		}
		if owner != actor.UserID {
			if err := auth.EnsureNewRecordOwner(ctx, tx, owner); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sales_initiative (id, name, description, owner_id, objective_metric, target_value, deadline, lead_rule, deal_rule, include_future, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'manual', $11)`,
			id, in.Name, in.Description, owner, in.Objective, in.Target, in.Deadline, leadRule, dealRule, include, by); err != nil {
			return fmt.Errorf("insert initiative: %w", err)
		}
		if err := writeRevision(ctx, tx, id, 1, in, by); err != nil {
			return err
		}
		out, err = readInitiative(ctx, tx, id)
		if err != nil {
			return err
		}
		return recordChange(ctx, tx, out, nil, []string{"definition"})
	})
	return out, err
}

// Update changes the definition. A live initiative keeps its baseline; the
// change is one more revision with the reason the caller gave.
func (s *Store) Update(ctx context.Context, id ids.UUID, in Definition, ifVersion *int64) (crmcontracts.SalesInitiative, error) {
	if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	if err := checkDefinition(in); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	var out crmcontracts.SalesInitiative
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := lockInitiative(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, table, id, ifVersion); err != nil {
			return err
		}
		leadRule, dealRule, err := ruleJSON(in)
		if err != nil {
			return err
		}
		owner := ids.UUID(before.OwnerId)
		if in.OwnerID != nil {
			owner = *in.OwnerID
		}
		include := before.IncludeFuture
		if in.IncludeFuture != nil {
			include = *in.IncludeFuture
		}
		if owner != ids.UUID(before.OwnerId) {
			if err := auth.EnsureAssignee(ctx, tx, owner); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE sales_initiative SET name = $2, description = $3, owner_id = $4, objective_metric = $5, target_value = $6, deadline = $7, lead_rule = $8, deal_rule = $9, include_future = $10, version = version + 1, updated_at = now() WHERE id = $1 AND archived_at IS NULL AND version = $11`,
			id, in.Name, in.Description, owner, in.Objective, in.Target, in.Deadline, leadRule, dealRule, include, versionOf(before))
		if err != nil {
			return fmt.Errorf("update initiative: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrVersionSkew
		}
		if err := writeRevision(ctx, tx, id, versionOf(before)+1, in, by); err != nil {
			return err
		}
		out, err = readInitiative(ctx, tx, id)
		if err != nil {
			return err
		}
		return recordChange(ctx, tx, out, &before, []string{"definition"})
	})
	return out, err
}

// Launch turns a draft live and freezes the member deals of this moment as
// the baseline. A launch never changes a record's ownership or visibility.
func (s *Store) Launch(ctx context.Context, id ids.UUID, ifVersion *int64) (crmcontracts.SalesInitiative, error) {
	if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	// The baseline freezes deals, so launching reads them.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	var out crmcontracts.SalesInitiative
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := lockInitiative(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, table, id, ifVersion); err != nil {
			return err
		}
		if before.Status != crmcontracts.SalesInitiativeStatusDraft {
			return fmt.Errorf("the initiative is already launched: %w", apperrors.ErrConflict)
		}
		members, err := s.membersTx(ctx, tx, before)
		if err != nil {
			return err
		}
		if len(members.Leads) == 0 && len(members.Deals) == 0 {
			return httperr.Validation("members", "empty", "include at least one lead or deal before launching")
		}
		if members.Truncated {
			return httperr.Validation("members", "too_many", "narrow the rules: a launch freezes at most "+strconv.Itoa(ruleReadLimit)+" records of each kind")
		}
		now := s.now().UTC()
		if _, err := tx.Exec(ctx, `INSERT INTO sales_initiative_baseline (initiative_id, deal_id, amount_minor_base, status, stage_id, captured_at)
			SELECT $1, d.id, d.amount_minor_base, d.status, d.stage_id, $3 FROM deal d WHERE d.id = ANY($2)`,
			id, members.Deals, now); err != nil {
			return fmt.Errorf("freeze the baseline: %w", err)
		}
		tag, err := tx.Exec(ctx, `UPDATE sales_initiative SET status = 'live', launch_at = $2, version = version + 1, updated_at = now() WHERE id = $1 AND archived_at IS NULL AND version = $3`, id, now, versionOf(before))
		if err != nil {
			return fmt.Errorf("launch initiative: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrVersionSkew
		}
		out, err = readInitiative(ctx, tx, id)
		if err != nil {
			return err
		}
		return recordChange(ctx, tx, out, &before, []string{"status", "baseline"})
	})
	return out, err
}

// Get reads one initiative.
func (s *Store) Get(ctx context.Context, id ids.UUID) (crmcontracts.SalesInitiative, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	var out crmcontracts.SalesInitiative
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readInitiative(ctx, tx, id)
		return err
	})
	return out, err
}

// List reads every live initiative, and the drafts, newest first.
func (s *Store) List(ctx context.Context, status *string) ([]crmcontracts.SalesInitiative, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.SalesInitiative
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{status}
		scope, err := initiativeScope(ctx, &args)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM sales_initiative WHERE archived_at IS NULL AND ($1::text IS NULL OR status = $1) AND `+scope+` ORDER BY created_at DESC, id DESC LIMIT 200`, args...)
		if err != nil {
			return fmt.Errorf("list initiatives: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.SalesInitiative, error) { return scanInitiative(row) })
		return err
	})
	if out == nil {
		out = []crmcontracts.SalesInitiative{}
	}
	return out, err
}

const columns = `id, name, description, owner_id, objective_metric, target_value, deadline, status, launch_at, lead_rule, deal_rule, include_future, version, created_at, updated_at`

// initiativeScope renders the reader's row scope over the initiative's owner:
// an initiative outside it is not found, as any row-scope miss is.
func initiativeScope(ctx context.Context, args *[]any) (string, error) {
	scope, err := auth.ScopeClauseFor(ctx, table, table, func(value any) int {
		*args = append(*args, value)
		return len(*args)
	})
	if err != nil {
		return "", err
	}
	if scope == "" {
		return "TRUE", nil
	}
	return scope, nil
}

func readInitiative(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.SalesInitiative, error) {
	args := []any{id}
	scope, err := initiativeScope(ctx, &args)
	if err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	out, err := scanInitiative(tx.QueryRow(ctx, `SELECT `+columns+` FROM sales_initiative WHERE id = $1 AND archived_at IS NULL AND `+scope, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.SalesInitiative{}, apperrors.ErrNotFound
	}
	return out, err
}

func lockInitiative(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.SalesInitiative, error) {
	if _, err := storekit.LockRow(ctx, tx, table, id, storekit.LiveOnly); err != nil {
		return crmcontracts.SalesInitiative{}, err
	}
	return readInitiative(ctx, tx, id)
}

func scanInitiative(row interface{ Scan(dest ...any) error }) (crmcontracts.SalesInitiative, error) {
	var out crmcontracts.SalesInitiative
	var id, owner ids.UUID
	var status, objective string
	var leadRule, dealRule []byte
	var version int64
	var deadline time.Time
	if err := row.Scan(&id, &out.Name, &out.Description, &owner, &objective, &out.TargetValue, &deadline, &status, &out.LaunchAt, &leadRule, &dealRule, &out.IncludeFuture, &version, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return out, err
	}
	out.Id = openapi_types.UUID(id)
	out.OwnerId = openapi_types.UUID(owner)
	out.ObjectiveMetric = crmcontracts.ReportingMetricID(objective)
	out.Status = crmcontracts.SalesInitiativeStatus(status)
	out.Deadline = openapi_types.Date{Time: deadline}
	rowVersion := crmcontracts.RowVersion(version)
	out.Version = &rowVersion
	var err error
	if out.LeadRule, err = ruleOf(leadRule); err != nil {
		return out, err
	}
	out.DealRule, err = ruleOf(dealRule)
	return out, err
}

func versionOf(initiative crmcontracts.SalesInitiative) int64 {
	if initiative.Version == nil {
		return 0
	}
	return int64(*initiative.Version)
}

func ruleOf(raw []byte) (*crmcontracts.SalesInitiativeRule, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rule crmcontracts.SalesInitiativeRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		return nil, fmt.Errorf("stored rule is not an object: %w", err)
	}
	return &rule, nil
}

func ruleJSON(in Definition) (lead, deal []byte, err error) {
	if in.LeadRule != nil {
		if lead, err = json.Marshal(in.LeadRule); err != nil {
			return nil, nil, err
		}
	}
	if in.DealRule != nil {
		if deal, err = json.Marshal(in.DealRule); err != nil {
			return nil, nil, err
		}
	}
	return lead, deal, nil
}

func checkDefinition(in Definition) error {
	if in.Name == "" {
		return httperr.Validation("name", "required", "name the initiative")
	}
	if in.Objective == "" {
		return httperr.Validation("objective_metric", "required", "choose the objective's metric")
	}
	if in.Target != nil && *in.Target <= 0 {
		return httperr.Validation("target_value", "invalid", "a target is one or more, or empty for tracking only")
	}
	if in.Deadline.IsZero() {
		return httperr.Validation("deadline", "required", "choose the deadline")
	}
	return nil
}

func writeRevision(ctx context.Context, tx pgx.Tx, id ids.UUID, revision int64, in Definition, by string) error {
	definition, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sales_initiative_revision (initiative_id, revision, definition, reason, created_by) VALUES ($1, $2, $3, $4, $5)`, id, revision, definition, in.Reason, by); err != nil {
		return fmt.Errorf("write initiative revision: %w", err)
	}
	return nil
}

// recordChange lands the write shape: the audit row and the paired event.
func recordChange(ctx context.Context, tx pgx.Tx, after crmcontracts.SalesInitiative, before *crmcontracts.SalesInitiative, changed []string) error {
	id := ids.UUID(after.Id)
	var auditID ids.UUID
	var err error
	if before == nil {
		auditID, err = storekit.Audit(ctx, tx, "create", object, id, nil, image(after))
	} else {
		auditID, err = storekit.Audit(ctx, tx, "update", object, id, image(*before), image(after))
	}
	if err != nil {
		return fmt.Errorf("audit initiative: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventSalesInitiativeUpdated{InitiativeId: after.Id, Status: crmcontracts.PublicEventSalesInitiativeUpdatedStatus(after.Status), ChangedFields: changed})
}

func image(initiative crmcontracts.SalesInitiative) map[string]any {
	return map[string]any{"name": initiative.Name, "status": initiative.Status, "objective_metric": initiative.ObjectiveMetric, "target_value": initiative.TargetValue, "deadline": initiative.Deadline, "owner_id": initiative.OwnerId, "version": initiative.Version}
}
