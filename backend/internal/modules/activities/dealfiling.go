// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

type DealFilingResolver interface {
	CompaniesOfRecords(ctx context.Context, tx pgx.Tx, records []ActivityLinkInput) ([]ids.UUID, error)
	Resolve(ctx context.Context, tx pgx.Tx, records []ActivityLinkInput) (crmcontracts.ActivityDealFilingOptions, error)
}

func (s *Store) WithDealFiling(resolver DealFilingResolver) *Store {
	s.dealFilingResolver = resolver
	return s
}

func (h Handlers) WithDealFiling(resolver DealFilingResolver) Handlers {
	h.store.WithDealFiling(resolver)
	return h
}

func (p *Provider) WithDealFiling(resolver DealFilingResolver) *Provider {
	p.store.WithDealFiling(resolver)
	return p
}

func (s *Store) DealFilingOptions(ctx context.Context, ref crmcontracts.ActivityDealFiling) (crmcontracts.ActivityDealFilingOptions, error) {
	out := crmcontracts.ActivityDealFilingOptions{}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return out, err
	}
	if err := validateDealFiling(string(crmcontracts.ActivityKindCall), &ref); err != nil {
		return out, err
	}
	if s.dealFilingResolver == nil {
		return out, errors.New("activity deal filing resolver is not configured")
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.dealFilingResolver.Resolve(ctx, tx, []ActivityLinkInput{{EntityType: string(ref.RecordType), EntityID: ids.UUID(ref.RecordId)}})
		return err
	})
	return out, err
}

func (h Handlers) GetActivityDealFiling(w http.ResponseWriter, r *http.Request, params crmcontracts.GetActivityDealFilingParams) {
	out, err := h.store.DealFilingOptions(r.Context(), crmcontracts.ActivityDealFiling{RecordType: params.RecordType, RecordId: params.RecordId})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func validateDealFiling(kind string, filing *crmcontracts.ActivityDealFiling) error {
	if filing == nil {
		return nil
	}
	if kind != string(crmcontracts.ActivityKindCall) && kind != KindMeeting {
		return httperr.Validation("deal_filing", "invalid_kind", "Deal filing applies only to calls and meetings.")
	}
	if !filing.RecordType.Valid() {
		return httperr.Validation("deal_filing", "invalid_record_type", "Choose a contact or company for deal filing.")
	}
	if filing.DealId != nil {
		if filing.Skip != nil && *filing.Skip {
			return httperr.Validation("deal_filing", "conflicting_choice", "Choose a deal or no deal, not both.")
		}
		if err := httperr.RequireBodyID("deal_id", ids.UUID(*filing.DealId)); err != nil {
			return err
		}
	}
	return httperr.RequireBodyID("record_id", ids.UUID(filing.RecordId))
}

func applyLoggedDealFiling(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if err := validateDealFiling(in.Kind, in.DealFiling); err != nil {
		return err
	}
	if in.Kind != string(crmcontracts.ActivityKindCall) && in.Kind != KindMeeting {
		return nil
	}
	imported, _ := storekit.ImportMark(ctx)
	if (imported != nil && *imported) || (in.SourceSystem != nil && strings.HasPrefix(*in.SourceSystem, provenance.ReservedSourceSystemPrefix)) {
		return nil
	}
	if in.Kind == KindMeeting && in.MeetingStatus != nil && *in.MeetingStatus != "held" {
		return nil
	}
	if err := validateFilingContext(ctx, tx, in); err != nil {
		return err
	}
	for _, link := range in.Links {
		if link.EntityType == "deal" || link.EntityType == "project" || link.EntityType == "lead" {
			return nil
		}
	}
	if in.DealFiling != nil && in.DealFiling.DealId != nil {
		in.Links = append(slices.Clone(in.Links), ActivityLinkInput{EntityType: "deal", EntityID: ids.UUID(*in.DealFiling.DealId)})
		return nil
	}
	if in.DealFiling != nil && in.DealFiling.Skip != nil && *in.DealFiling.Skip {
		return nil
	}
	if in.dealFilingResolver == nil {
		if in.DealFiling != nil {
			return errors.New("activity deal filing resolver is not configured")
		}
		return nil
	}
	records := in.Links
	if in.DealFiling != nil {
		records = []ActivityLinkInput{{EntityType: string(in.DealFiling.RecordType), EntityID: ids.UUID(in.DealFiling.RecordId)}}
	}
	options, err := in.dealFilingResolver.Resolve(ctx, tx, records)
	if err != nil {
		return err
	}
	if options.ChoiceRequired {
		if in.DealFiling != nil {
			return httperr.Validation("deal_filing", "choose_deal", "Choose an open deal or choose none.")
		}
		return nil
	}
	if options.SuggestedDealId != nil {
		in.Links = append(slices.Clone(in.Links), ActivityLinkInput{EntityType: "deal", EntityID: ids.UUID(*options.SuggestedDealId)})
	}
	return nil
}

func validateFilingContext(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	ref := in.DealFiling
	if ref == nil {
		return nil
	}
	for _, link := range in.Links {
		if link.EntityType == string(ref.RecordType) && link.EntityID == ids.UUID(ref.RecordId) {
			return nil
		}
	}
	if ref.RecordType == "company" && in.dealFilingResolver != nil {
		companies, err := in.dealFilingResolver.CompaniesOfRecords(ctx, tx, in.Links)
		if err != nil {
			return err
		}
		if slices.Contains(companies, ids.UUID(ref.RecordId)) {
			return nil
		}
	}
	return httperr.Validation("deal_filing", "unrelated_record", "Choose the linked contact or its current company.")
}

func (s *Store) fileUpdatedActivity(ctx context.Context, tx pgx.Tx, current crmcontracts.Activity, in UpdateActivityInput) (crmcontracts.Activity, error) {
	if in.DealFiling == nil && in.MeetingStatus == nil && in.CallOutcome == nil {
		return current, nil
	}
	filing := LogActivityInput{Kind: string(current.Kind), DealFiling: in.DealFiling,
		SourceSystem: current.SourceSystem, dealFilingResolver: s.dealFilingResolver}
	if current.MeetingStatus != nil {
		filing.MeetingStatus = new(string(*current.MeetingStatus))
	}
	for _, kind := range []string{"deal", "project", "lead"} {
		if filed, err := hasLinkOfType(ctx, tx, ids.From[ids.ActivityKind](ids.UUID(current.Id)), kind); err != nil || filed {
			return current, err
		}
	}
	var links []crmcontracts.ActivityLink
	if current.Links != nil {
		links = *current.Links
	}
	for _, link := range links {
		filing.Links = append(filing.Links, ActivityLinkInput{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
	}
	if err := applyLoggedDealFiling(ctx, tx, &filing); err != nil {
		return current, err
	}
	added := filing.Links[len(links):]
	if len(added) == 0 {
		return current, nil
	}
	if err := storekit.LockLastActivityTargets(ctx, tx, lastActivityTargets(added)); err != nil {
		return current, err
	}
	for _, link := range added {
		if _, err := RelinkActivityInTx(ctx, tx, ids.From[ids.ActivityKind](ids.UUID(current.Id)), RelinkActivityInput{
			EntityType: link.EntityType, EntityID: link.EntityID, unfiledOnly: true,
		}); err != nil {
			return current, err
		}
	}
	return readActivityAt(ctx, tx, ids.From[ids.ActivityKind](ids.UUID(current.Id)), storekit.LiveOnly, s.now())
}
