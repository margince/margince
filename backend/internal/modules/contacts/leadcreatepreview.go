// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// LeadCreationPreview resolves creation defaults and duplicate handling before approval.
type LeadCreationPreview struct {
	Input        CreateLeadInput
	Existing     *crmcontracts.Lead
	CustomFields map[string]any
	Score        int
	ReviewLead   *crmcontracts.Lead
}

// PreviewLeadCreation uses the caller's creation authority. A reviewer must not
// substitute their identity for the actor whose defaults the approved call uses.
func (s *Store) PreviewLeadCreation(ctx context.Context, in CreateLeadInput) (LeadCreationPreview, error) {
	creator, ok := principal.Actor(ctx)
	if !ok {
		return LeadCreationPreview{}, apperrors.ErrPermissionDenied
	}
	return s.PreviewLeadCreationForActor(ctx, in, creator)
}

// PreviewLeadCreationForActor uses creator identity for defaults only; every read and admission uses the reviewer.
func (s *Store) PreviewLeadCreationForActor(ctx context.Context, in CreateLeadInput, creator principal.Principal) (LeadCreationPreview, error) {
	var out LeadCreationPreview
	if err := auth.Require(ctx, leadEntity, principal.ActionCreate); err != nil {
		return out, err
	}
	in, err := s.previewLeadContactDefaults(ctx, in)
	if err != nil {
		return out, err
	}
	in, _, err = s.readyLeadCreate(principal.WithActor(ctx, creator), in)
	if err != nil {
		return out, err
	}
	active, err := s.activeColumns(ctx, leadEntity)
	if err != nil {
		return out, err
	}
	custom, err := storekit.CreateFieldValues(active, in.CustomFields)
	if err != nil {
		return out, err
	}
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		existing, err := replayedLead(ctx, tx, in, active)
		if err != nil {
			return err
		}
		if existing != nil {
			out = LeadCreationPreview{Existing: existing}
			return nil
		}
		if err := admitLeadCreate(ctx, tx, in); err != nil {
			return err
		}
		in, err = admitLeadOrigin(ctx, tx, in)
		if err != nil {
			return err
		}
		if err := storekit.RefuseUnknownSeat(ctx, tx, in.Author); err != nil {
			return err
		}
		out.Input, out.CustomFields = in, custom
		score, err := initialLeadScore(ctx, tx, in)
		if err != nil {
			return err
		}
		out.Score = score.Score
		out.ReviewLead, err = s.previewCreatedLeadMatch(ctx, tx, in, active)
		return err
	})
	if err != nil {
		return LeadCreationPreview{}, err
	}
	return out, nil
}

func initialLeadScore(ctx context.Context, tx pgx.Tx, in CreateLeadInput) (LeadScoring, error) {
	intents, err := LoadOriginIntents(ctx, tx)
	if err != nil {
		return LeadScoring{}, err
	}
	// New leads have no behavioral signals to age.
	return ScoreLeadDetail(deref(in.Title), intents.Of(in.Origin), nil, time.Time{}), nil
}

func (s *Store) previewCreatedLeadMatch(ctx context.Context, tx pgx.Tx, in CreateLeadInput, active []fieldcatalog.Column) (*crmcontracts.Lead, error) {
	if deref(in.FullName) == "" {
		return nil, nil //nolint:nilnil // No name means there is no fuzzy review candidate.
	}
	proposed := crmcontracts.Lead{FullName: in.FullName, CompanyName: in.CompanyName}
	if in.Email != nil {
		proposed.Email = new(openapi_types.Email(*in.Email))
	}
	match, err := s.fuzzyLead(ctx, tx, proposed)
	if err != nil || !match.found {
		return nil, err
	}
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.EnsureVisible(ctx, tx, leadEntity, match.id.UUID); err != nil {
		return nil, err
	}
	lead, err := readLead(ctx, tx, ids.From[ids.LeadKind](match.id.UUID), storekit.LiveOnly, active)
	if err != nil {
		return nil, err
	}
	return &lead, nil
}

func (s *Store) previewLeadContactDefaults(ctx context.Context, in CreateLeadInput) (CreateLeadInput, error) {
	if in.FromContactID == nil {
		return in, nil
	}
	contact, err := s.GetContact(ctx, *in.FromContactID, storekit.LiveOnly)
	if err != nil {
		return CreateLeadInput{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return CreateLeadInput{}, apperrors.ErrPermissionDenied
	}
	inherited := map[string]*string{
		contactNameColumn: in.FullName, fieldTitle: in.Title, fieldEmail: in.Email, "primary_email": in.Email, contactEmailsField: in.Email,
		"employer": in.CompanyName, "social": in.LinkedInURL,
	}
	for _, field := range auth.MaskedFields(actor, contactEntity, contact.Writable != nil && *contact.Writable) {
		supplied, relevant := inherited[field]
		if relevant && strings.TrimSpace(deref(supplied)) == "" {
			return CreateLeadInput{}, apperrors.ErrPermissionDenied
		}
	}
	return leadFromContactDefaults(in, contact), nil
}
