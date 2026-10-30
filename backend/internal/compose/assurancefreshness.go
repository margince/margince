// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:assurance-freshness consumer: a change to what the input check reads
// marks the last reading out of date, so no screen shows it as current.
//
// It marks and never scans. The nightly pass stays the only scan, and the next
// one clears what it read (assurance/freshness.go). Which events count, and how
// far each reaches, is assurance.InputChangeOf: this file only finds the deal
// an event is about.
//
// Coalesced: one read of the stream is one statement, one row per distinct
// subject, so a burst of edits to one deal writes one row. A redelivered event
// marks its subject again, which can make it out of date but never current, so
// events.Dedupe in front of it is a cache and not the guarantee.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// assuranceFreshnessActor names this consumer as the principal of its writes.
const assuranceFreshnessActor = "system:assurance-freshness"

// AssuranceFreshness marks the input check's reading out of date.
type AssuranceFreshness struct {
	db       *database.DB
	store    *assurance.Store
	identity *identity.Service
}

// NewAssuranceFreshness builds the consumer over the installation's handle.
func NewAssuranceFreshness(pool *pgxpool.Pool, ident *identity.Service) *AssuranceFreshness {
	db := database.Bind(pool, ident.InstallationWorkspace)
	return &AssuranceFreshness{db: db, store: assurance.NewStore(db), identity: ident}
}

// HandleEvent is the single-entry path; it marks exactly what a batch would.
func (f *AssuranceFreshness) HandleEvent(ctx context.Context, env events.Envelope) error {
	return f.HandleBatch(ctx, []events.Envelope{env})
}

// HandleBatch marks every subject the envelopes changed, in one transaction.
func (f *AssuranceFreshness) HandleBatch(ctx context.Context, envs []events.Envelope) error {
	changes, err := inputChangesOf(envs)
	if err != nil || changes.empty() {
		return err
	}
	// The envelope carries no tenant, and the subscriber binds none.
	ws, err := f.identity.InstallationWorkspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: assuranceFreshnessActor})
	return f.db.Tx(ctx, func(tx pgx.Tx) error {
		subjects, err := changes.withContractDeals(ctx, tx)
		if err != nil {
			return err
		}
		return f.store.MarkInputsChanged(ctx, tx, subjects)
	})
}

// inputChanges is what a batch changed: the subjects it names outright, and
// the contracts whose deal has to be looked up.
type inputChanges struct {
	subjects  []assurance.ChangedSubject
	contracts []ids.UUID
}

func (c inputChanges) empty() bool { return len(c.subjects) == 0 && len(c.contracts) == 0 }

// inputChangesOf sorts each envelope by its declared scope. An undeclared type
// and a declared no-op mark nothing.
func inputChangesOf(envs []events.Envelope) (inputChanges, error) {
	var out inputChanges
	for _, env := range envs {
		change, ok := assurance.InputChangeOf(env.Type)
		if !ok {
			continue
		}
		switch change.Scope {
		case assurance.ScopeWorkspace:
			out.subjects = append(out.subjects, assurance.WorkspaceChanged())
		case assurance.ScopeDeal:
			if err := out.addDealOf(env); err != nil {
				return inputChanges{}, err
			}
		case assurance.ScopeNone:
		}
	}
	return out, nil
}

// addDealOf finds the deal an envelope is about: its entity, the deal its
// payload names, or, for a contract, the deal the contract came from.
func (c *inputChanges) addDealOf(env events.Envelope) error {
	if env.Entity.Type == (crmcontracts.PublicEventDealUpdated{}).EntityType() {
		c.subjects = append(c.subjects, assurance.DealChanged(env.Entity.ID))
		return nil
	}
	var payload struct {
		DealID string `json:"deal_id"`
	}
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return fmt.Errorf("assurance freshness: %s payload: %w", env.Type, err)
		}
	}
	if payload.DealID != "" {
		deal, err := ids.Parse(payload.DealID)
		if err != nil {
			return fmt.Errorf("assurance freshness: %s names a deal id it cannot parse: %w", env.Type, err)
		}
		c.subjects = append(c.subjects, assurance.DealChanged(deal))
		return nil
	}
	if env.Entity.Type == (crmcontracts.PublicEventContractUpdated{}).EntityType() {
		c.contracts = append(c.contracts, env.Entity.ID)
	}
	return nil
}

// withContractDeals adds the deal of each contract. A contract that came from
// no deal changes no deal's reading.
func (c inputChanges) withContractDeals(ctx context.Context, tx pgx.Tx) ([]assurance.ChangedSubject, error) {
	if len(c.contracts) == 0 {
		return c.subjects, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT deal_id FROM contract WHERE id = ANY($1) AND deal_id IS NOT NULL`, c.contracts)
	if err != nil {
		return nil, fmt.Errorf("assurance freshness: reading the contracts' deals: %w", err)
	}
	deals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("assurance freshness: reading the contracts' deals: %w", err)
	}
	out := c.subjects
	for _, deal := range deals {
		out = append(out, assurance.DealChanged(deal))
	}
	return out, nil
}
