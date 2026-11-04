// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Margince opening the deals Deal Scout found, as the company owner's agent.
// A company with no owner, an owner who left, or an owner who switched
// deal_open off keeps the suggestion for a human to decide.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scoutDealOpener opens the open_deal suggestions whose company has an owner.
type scoutDealOpener struct {
	pool       *pgxpool.Pool
	deals      *deals.Store
	users      *identity.Service
	activities *activities.Store
	log        *slog.Logger
}

func newScoutDealOpener(pool *pgxpool.Pool, log *slog.Logger) scoutDealOpener {
	return scoutDealOpener{
		pool:       pool,
		deals:      deals.NewStore(InstallationDB(pool), DealsInstallation()).WithSuggestionEffects(dealSuggestionEffects{}),
		users:      identity.NewService(pool),
		activities: activities.NewStore(InstallationDB(pool)),
		log:        log,
	}
}

// openableSuggestion is one suggestion and the owner of its company.
type openableSuggestion struct {
	id, owner ids.UUID
}

// OpenWorkspace opens every suggestion it may in the workspace bound in ctx,
// and reports how many deals it opened. One that cannot open does not stop
// the others.
func (o scoutDealOpener) OpenWorkspace(ctx context.Context) (int, error) {
	var due []openableSuggestion
	err := database.WithWorkspaceTx(ctx, o.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.id, c.owner_id FROM deal_suggestion s JOIN company c ON c.id = s.company_id
			 WHERE s.state = 'open' AND s.kind = $1 AND c.owner_id IS NOT NULL AND c.archived_at IS NULL
			 ORDER BY s.created_at, s.id`, deals.SuggestionKindOpenDeal)
		if err != nil {
			return fmt.Errorf("compose: reading the suggestions to open: %w", err)
		}
		due, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (openableSuggestion, error) {
			var s openableSuggestion
			return s, row.Scan(&s.id, &s.owner)
		})
		return err
	})
	if err != nil {
		return 0, err
	}
	opened := 0
	for _, s := range due {
		deal, err := o.openOne(ctx, s)
		if err != nil {
			o.log.WarnContext(ctx, "Margince could not open a suggested deal", "suggestion_id", s.id, "error", err)
			continue
		}
		if !deal.IsZero() {
			opened++
		}
	}
	return opened, nil
}

// openOne opens one suggestion as its company owner's agent. An owner who
// left answers not found, and the suggestion stays.
func (o scoutDealOpener) openOne(ctx context.Context, s openableSuggestion) (ids.UUID, error) {
	ownerCtx, err := ownersAgent(ctx, o.users, s.owner, marginceActorID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, err
	}
	suggestion, err := o.deals.GetSuggestion(ownerCtx, s.id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, err
	}
	basis := scoutBasis(suggestion)
	return o.deals.OpenSuggestedDeal(ownerCtx, deals.OpenSuggestedDealInput{
		SuggestionID: s.id, Owner: s.owner, Basis: basis,
		SwitchOn: func(ctx context.Context, tx pgx.Tx) (bool, error) {
			mode, err := approvals.AutoApplyModeTx(ctx, tx, s.owner, deals.DealOpenKind)
			return mode == approvals.ModeAuto, err
		},
		Record: func(ctx context.Context, tx pgx.Tx, deal, _ ids.UUID) error {
			return o.recordOnTimeline(ctx, tx, deal, suggestion.CompanyName, basis)
		},
	})
}

// scoutBasis is the receipt's reason: each cited item as the reader knows it,
// with its date, newest first as the suggestion lists them.
func scoutBasis(s deals.Suggestion) string {
	parts := make([]string, 0, len(s.Evidence))
	for _, item := range s.Evidence {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = item.Kind
		}
		parts = append(parts, fmt.Sprintf("%s (%s, %s)", title, item.Kind, item.OccurredAt.UTC().Format(time.DateOnly)))
	}
	return strings.Join(parts, "; ")
}

// recordOnTimeline writes the opening on the deal's timeline as a note. Its
// origin keeps it out of every recency clock.
func (o scoutDealOpener) recordOnTimeline(ctx context.Context, tx pgx.Tx, deal ids.UUID, company, basis string) error {
	subject := "Margince opened a deal for " + company
	body := basis + ". Undo it from Home if this is not a deal."
	_, _, err := o.activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{{EntityType: entityTypeDeal, EntityID: deal}},
	})
	if err != nil {
		return fmt.Errorf("compose: writing the opening on the deal's timeline: %w", err)
	}
	return nil
}
