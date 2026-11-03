// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:assurance-freshness consumer: a change to what the input check reads
// marks the last reading out of date, so no screen shows it as current.
//
// It marks and never scans. The nightly pass stays the only scan, and the next
// complete one clears what it read (assurance/freshness.go). Which events
// count, and how far each reaches, is assurance.InputChangeOf. This file only
// finds the deal or buying signal an event is about.
//
// Coalesced: one read of the stream is one statement, one row per distinct
// subject, so a burst of edits to one deal writes one row. A redelivered event
// marks its subject again, which can make it out of date but never current.
// So events.Dedupe in front of it is a cache and not the guarantee.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

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

// freshnessEntityContract is the entity a contract event is named for.
const freshnessEntityContract = "contract"

// assuranceFreshnessActor names this consumer as the principal of its writes.
const assuranceFreshnessActor = "system:assurance-freshness"

// AssuranceFreshness marks the input check's reading out of date.
type AssuranceFreshness struct {
	db       *database.DB
	store    *assurance.Store
	identity *identity.Service
	// rescans queues a one-deal pass for each changed deal. Nil marks only.
	rescans assuranceRescanEnqueuer
}

// WithRescans queues a one-deal input check for every deal the marker marks,
// in the transaction that marks it.
func (f *AssuranceFreshness) WithRescans(rescans assuranceRescanEnqueuer) *AssuranceFreshness {
	wired := *f
	wired.rescans = rescans
	return &wired
}

// NewAssuranceFreshness builds the consumer over the installation's handle.
func NewAssuranceFreshness(pool *pgxpool.Pool, ident *identity.Service) *AssuranceFreshness {
	db := database.Bind(pool, ident.InstallationWorkspace)
	return &AssuranceFreshness{db: db, store: assurance.NewStore(db), identity: ident}
}

// HandleEvent is the single-entry path; it marks what a batch of one would.
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
		subjects, err := changes.resolve(ctx, tx)
		if err != nil {
			return err
		}
		if err := f.store.MarkInputsChanged(ctx, tx, subjects); err != nil {
			return err
		}
		return f.queueRescans(ctx, tx, ws.UUID, subjects)
	})
}

// queueRescans queues one pass per changed deal, at most
// assuranceRescanBatchLimit of them and never past the workspace's backlog
// limit (assuranceRescanRoom). A workspace nobody started is not
// checked, so nothing is queued there. A change that reaches every deal queues
// nothing: that is the nightly pass's work.
func (f *AssuranceFreshness) queueRescans(
	ctx context.Context, tx pgx.Tx, workspace ids.UUID, subjects []assurance.ChangedSubject,
) error {
	if f.rescans == nil {
		return nil
	}
	var changed []ids.UUID
	for _, subject := range subjects {
		if subject.Scope == assurance.ScopeDeal && !slices.Contains(changed, subject.ID) {
			changed = append(changed, subject.ID)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	enrolled, err := f.store.EverRunTx(ctx, tx)
	if err != nil || !enrolled {
		return err
	}
	room, err := assuranceRescanRoom(ctx, tx, workspace)
	if err != nil {
		return err
	}
	slices.SortFunc(changed, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
	for _, deal := range changed[:min(len(changed), room)] {
		args := AssuranceDealRescanArgs{Workspace: workspace, Deal: deal}
		if err := f.rescans.EnqueueTx(ctx, tx, args, assuranceRescanInsertOpts()); err != nil {
			return err
		}
	}
	return nil
}

// inputChanges is what a batch changed: the subjects it names outright, and
// the records whose subjects have to be looked up.
type inputChanges struct {
	subjects  []assurance.ChangedSubject
	contracts []ids.UUID
	// activities reach the open deals they are linked to.
	activities []ids.UUID
	// companies and suggestions reach the buying signals they bear on.
	companies []ids.UUID
	// suggestionAudits are the audit rows of suggestion events, which name the
	// suggestion where the event names none.
	suggestionAudits []ids.UUID
}

func (c inputChanges) empty() bool {
	return len(c.subjects) == 0 && len(c.contracts) == 0 && len(c.activities) == 0 &&
		len(c.companies) == 0 && len(c.suggestionAudits) == 0
}

// inputChangesOf sorts each envelope by its declared scope. An undeclared type
// and a declared no-op mark nothing.
func inputChangesOf(envs []events.Envelope) (inputChanges, error) {
	var out inputChanges
	for _, env := range envs {
		change, ok := assurance.InputChangeOf(env.Type)
		if !ok {
			continue
		}
		var err error
		switch change.Scope {
		case assurance.ScopeWorkspace:
			out.subjects = append(out.subjects, assurance.WorkspaceChanged())
		case assurance.ScopeDeal:
			err = out.addDealOf(env)
		case assurance.ScopeDealActivity:
			out.activities = append(out.activities, env.Entity.ID)
		case assurance.ScopeSignal:
			err = out.addSignalsOf(env)
		case assurance.ScopeNone:
		}
		if err != nil {
			return inputChanges{}, err
		}
	}
	return out, nil
}

// inputReferences holds the ids a payload may name besides its entity. Those
// are the deal of a contract or offer, and the record a merge kept.
type inputReferences struct {
	DealID       string `json:"deal_id"`
	MergedIntoID string `json:"merged_into_id"`
}

func referencesOf(env events.Envelope) (inputReferences, error) {
	var out inputReferences
	if len(env.Payload) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(env.Payload, &out); err != nil {
		return out, fmt.Errorf("assurance freshness: %s payload: %w", env.Type, err)
	}
	return out, nil
}

// parseReference is one payload id, which a malformed event must not pass as
// no id at all.
func parseReference(env events.Envelope, field, raw string) (ids.UUID, error) {
	id, err := ids.Parse(raw)
	if err != nil {
		return ids.Nil, fmt.Errorf("assurance freshness: %s names a %s it cannot parse: %w", env.Type, field, err)
	}
	return id, nil
}

// addDealOf finds the deals an envelope is about. That is its entity and the
// deal a merge kept, the deal its payload names, or a contract's deal.
func (c *inputChanges) addDealOf(env events.Envelope) error {
	refs, err := referencesOf(env)
	if err != nil {
		return err
	}
	if env.Entity.Type == tableDeal {
		c.subjects = append(c.subjects, assurance.DealChanged(env.Entity.ID))
		if refs.MergedIntoID != "" {
			into, err := parseReference(env, "merged_into_id", refs.MergedIntoID)
			if err != nil {
				return err
			}
			c.subjects = append(c.subjects, assurance.DealChanged(into))
		}
		return nil
	}
	if refs.DealID != "" {
		deal, err := parseReference(env, "deal_id", refs.DealID)
		if err != nil {
			return err
		}
		c.subjects = append(c.subjects, assurance.DealChanged(deal))
		return nil
	}
	if env.Entity.Type == freshnessEntityContract {
		c.contracts = append(c.contracts, env.Entity.ID)
	}
	return nil
}

// addSignalsOf sorts an envelope that bears on buying signals by what it
// names. That is a signal, a company and the one a merge kept, or a suggestion.
func (c *inputChanges) addSignalsOf(env events.Envelope) error {
	switch env.Entity.Type {
	case tableSignal:
		c.subjects = append(c.subjects, assurance.SignalChanged(env.Entity.ID))
	case tableCompany:
		c.companies = append(c.companies, env.Entity.ID)
		refs, err := referencesOf(env)
		if err != nil || refs.MergedIntoID == "" {
			return err
		}
		into, err := parseReference(env, "merged_into_id", refs.MergedIntoID)
		if err != nil {
			return err
		}
		c.companies = append(c.companies, into)
	case "":
		// deal_suggestion events name no entity; the audit row names the suggestion.
		c.suggestionAudits = append(c.suggestionAudits, env.Trace.AuditLogID)
	default:
		return fmt.Errorf("assurance freshness: %s bears on buying signals through a %s, which this consumer cannot follow",
			env.Type, env.Entity.Type)
	}
	return nil
}

// resolve adds the looked-up subjects to the ones the batch named outright.
func (c inputChanges) resolve(ctx context.Context, tx pgx.Tx) ([]assurance.ChangedSubject, error) {
	out := c.subjects
	lookups := []struct {
		subject func(ids.UUID) assurance.ChangedSubject
		keys    []ids.UUID
		sql     string
	}{
		{assurance.DealChanged, c.contracts, contractDealsSQL},
		{assurance.DealChanged, c.activities, movedActivityDealsSQL},
		{assurance.SignalChanged, c.companies, companySignalsSQL},
		{assurance.SignalChanged, c.suggestionAudits, suggestionSignalsSQL},
	}
	for _, lookup := range lookups {
		if len(lookup.keys) == 0 {
			continue
		}
		rows, err := tx.Query(ctx, lookup.sql, lookup.keys)
		if err != nil {
			return nil, fmt.Errorf("assurance freshness: finding what the changes reach: %w", err)
		}
		found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return nil, fmt.Errorf("assurance freshness: finding what the changes reach: %w", err)
		}
		for _, id := range found {
			out = append(out, lookup.subject(id))
		}
	}
	return out, nil
}

// contractDealsSQL is the deal of each contract. A contract that came from no
// deal changes no deal's reading.
const contractDealsSQL = `SELECT deal_id FROM contract WHERE id = ANY($1) AND deal_id IS NOT NULL`

// movedActivityDealsSQL is each open deal the activities are linked to whose
// viability fingerprint no longer matches the one the last run stored. The
// fingerprint is the same one the check stores and the forecast compares, so
// an activity edit the check does not read marks no deal.
var movedActivityDealsSQL = `SELECT DISTINCT d.id
	  FROM activity_link l
	  JOIN deal d ON d.id = l.deal_id
	  LEFT JOIN assurance_deal_receipt r ON r.deal_id = d.id
	 WHERE l.activity_id = ANY($1) AND d.status = 'open' AND d.archived_at IS NULL
	   AND r.input_fingerprint IS DISTINCT FROM ` + viabilityFingerprintSQL("d")

// companySignalsSQL is the buying signals resolved to each company.
var companySignalsSQL = `SELECT id FROM signal
	 WHERE resolved_company_id = ANY($1) AND kind = '` + string(crmcontracts.SignalKindNewOpportunity) + `'`

// suggestionSignalsSQL is the buying signals each suggestion cites, found
// through the audit row its event links to.
const suggestionSignalsSQL = `SELECT ev.signal_id
	  FROM audit_log a
	  JOIN deal_suggestion_evidence ev ON ev.suggestion_id = a.entity_id
	 WHERE a.id = ANY($1) AND a.entity_type = 'deal_suggestion' AND ev.signal_id IS NOT NULL`
