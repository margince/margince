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
	"unicode/utf8"

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
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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

// openableSuggestion is one suggestion, the owner of its company, and the
// company's latest closed deal when that deal was lost.
type openableSuggestion struct {
	id, owner ids.UUID
	lost      *ids.UUID
	lostOwner *ids.UUID
}

// OpenWorkspace opens or reopens a deal for every suggestion it may in the
// workspace bound in ctx, and reports how many it did. A company whose latest
// closed deal was lost gets that deal back, never a second one beside it. One
// that cannot act does not stop the others.
func (o scoutDealOpener) OpenWorkspace(ctx context.Context) (int, error) {
	var due []openableSuggestion
	err := database.WithWorkspaceTx(ctx, o.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.id, c.owner_id, latest.id, latest.owner_id
			  FROM deal_suggestion s JOIN company c ON c.id = s.company_id
			  LEFT JOIN LATERAL (SELECT d.id, d.owner_id FROM deal d
			          WHERE d.company_id = s.company_id AND d.archived_at IS NULL AND d.status <> 'open'
			          ORDER BY d.closed_at DESC NULLS LAST, d.id DESC LIMIT 1) closed ON true
			  LEFT JOIN deal latest ON latest.id = closed.id AND latest.status = 'lost'
			 WHERE s.state = 'open' AND s.kind = $1 AND c.archived_at IS NULL
			   AND (c.owner_id IS NOT NULL OR latest.owner_id IS NOT NULL)
			 ORDER BY s.created_at, s.id`, deals.SuggestionKindOpenDeal)
		if err != nil {
			return fmt.Errorf("compose: reading the suggestions to open: %w", err)
		}
		due, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (openableSuggestion, error) {
			var s openableSuggestion
			var owner *ids.UUID
			err := row.Scan(&s.id, &owner, &s.lost, &s.lostOwner)
			if owner != nil {
				s.owner = *owner
			}
			return s, err
		})
		return err
	})
	if err != nil {
		return 0, err
	}
	done := 0
	for _, s := range due {
		acted, err := o.actOn(ctx, s)
		if err != nil {
			o.log.WarnContext(ctx, "Margince could not act on a suggested deal", "suggestion_id", s.id, "error", err)
			continue
		}
		if acted {
			done++
		}
	}
	return done, nil
}

// actOn reopens the company's lost deal when it has one, and otherwise opens
// a deal for the company's owner.
func (o scoutDealOpener) actOn(ctx context.Context, s openableSuggestion) (bool, error) {
	if s.lost != nil {
		if s.lostOwner == nil {
			return false, nil
		}
		return o.reviveOne(ctx, s.id, ids.From[ids.DealKind](*s.lost), *s.lostOwner)
	}
	if s.owner.IsZero() {
		return false, nil
	}
	deal, err := o.openOne(ctx, s)
	return !deal.IsZero(), err
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
		EvidenceHolds: scoutEvidenceHolds, Amount: scoutStatedAmount,
		SwitchOn: func(ctx context.Context, tx pgx.Tx) (bool, error) {
			mode, err := approvals.AutoApplyModeTx(ctx, tx, s.owner, deals.DealOpenKind)
			return mode == approvals.ModeAuto, err
		},
		Record: func(ctx context.Context, tx pgx.Tx, deal, _ ids.UUID) error {
			return o.recordOnTimeline(ctx, tx, deal, suggestion.CompanyName, basis)
		},
		AsOwner: ownersAgentTx(o.users, s.owner, marginceActorID),
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
	subject := fmt.Sprintf(systemtext.Copy("deal.opened", textlang.English), company)
	body := fmt.Sprintf(systemtext.Copy("deal.opened_body", textlang.English), basis)
	_, _, err := o.activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("deal.opened", systemtext.Text(company)), Body: systemtext.Sentence("deal.opened_body", systemtext.Excerpt(0, utf8.RuneCountInString(basis)))},
		Kind:       string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{{EntityType: entityTypeDeal, EntityID: deal}},
	})
	if err != nil {
		return fmt.Errorf("compose: writing the opening on the deal's timeline: %w", err)
	}
	return nil
}
