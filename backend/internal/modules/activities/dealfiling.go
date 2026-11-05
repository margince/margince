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

type dealFilingContext struct {
	kind          string
	meetingStatus *string
	source        *string
	choice        *crmcontracts.ActivityDealFiling
	links         []ActivityLinkInput
	resolver      DealFilingResolver
}

func (in *LogActivityInput) applyDealFiling(ctx context.Context, tx pgx.Tx) error {
	filing := dealFilingContext{
		kind: in.Kind, meetingStatus: in.MeetingStatus,
		source: in.SourceSystem, choice: in.DealFiling, links: in.Links, resolver: in.dealFilingResolver,
	}
	if err := applyLoggedDealFiling(ctx, tx, &filing); err != nil {
		return err
	}
	in.Links = filing.links
	return nil
}

func applyLoggedDealFiling(ctx context.Context, tx pgx.Tx, in *dealFilingContext) error {
	if err := validateDealFiling(in.kind, in.choice); err != nil {
		return err
	}
	if !in.shouldFile(ctx) {
		return nil
	}
	if err := validateFilingContext(ctx, tx, in); err != nil {
		return err
	}
	for _, link := range in.links {
		if link.EntityType == "deal" || link.EntityType == "project" || link.EntityType == "lead" {
			return nil
		}
	}
	if in.choice != nil && in.choice.DealId != nil {
		in.links = append(slices.Clone(in.links), ActivityLinkInput{EntityType: "deal", EntityID: ids.UUID(*in.choice.DealId)})
		return nil
	}
	if in.choice != nil && in.choice.Skip != nil && *in.choice.Skip {
		return nil
	}
	options, err := in.suggestedDeal(ctx, tx)
	if err != nil {
		return err
	}
	if options.SuggestedDealId != nil {
		in.links = append(slices.Clone(in.links), ActivityLinkInput{EntityType: "deal", EntityID: ids.UUID(*options.SuggestedDealId)})
	}
	return nil
}

func (in *dealFilingContext) shouldFile(ctx context.Context) bool {
	if in.kind != string(crmcontracts.ActivityKindCall) && in.kind != KindMeeting {
		return false
	}
	imported, _ := storekit.ImportMark(ctx)
	if (imported != nil && *imported) || (in.source != nil && strings.HasPrefix(*in.source, provenance.ReservedSourceSystemPrefix)) {
		return false
	}
	return in.kind != KindMeeting || in.meetingStatus == nil || *in.meetingStatus == string(crmcontracts.ActivityMeetingStatusHeld)
}

func (in *dealFilingContext) suggestedDeal(ctx context.Context, tx pgx.Tx) (crmcontracts.ActivityDealFilingOptions, error) {
	var empty crmcontracts.ActivityDealFilingOptions
	if in.resolver == nil {
		if in.choice != nil {
			return empty, errors.New("activity deal filing resolver is not configured")
		}
		return empty, nil
	}
	records := in.links
	if in.choice != nil {
		records = []ActivityLinkInput{{EntityType: string(in.choice.RecordType), EntityID: ids.UUID(in.choice.RecordId)}}
	}
	options, err := in.resolver.Resolve(ctx, tx, records)
	if err != nil {
		return empty, err
	}
	if options.ChoiceRequired {
		if in.choice != nil {
			return empty, httperr.Validation("deal_filing", "choose_deal", "Choose an open deal or choose none.")
		}
		return empty, nil
	}
	return options, nil
}

func validateFilingContext(ctx context.Context, tx pgx.Tx, in *dealFilingContext) error {
	ref := in.choice
	if ref == nil {
		return nil
	}
	for _, link := range in.links {
		if link.EntityType == string(ref.RecordType) && link.EntityID == ids.UUID(ref.RecordId) {
			return nil
		}
	}
	if ref.RecordType == linkEntityCompany && in.resolver != nil {
		companies, err := in.resolver.CompaniesOfRecords(ctx, tx, in.links)
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
	filing := dealFilingContext{
		kind: string(current.Kind), choice: in.DealFiling,
		source: current.SourceSystem, resolver: s.dealFilingResolver,
	}
	if current.MeetingStatus != nil {
		filing.meetingStatus = new(string(*current.MeetingStatus))
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
		filing.links = append(filing.links, ActivityLinkInput{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
	}
	if err := applyLoggedDealFiling(ctx, tx, &filing); err != nil {
		return current, err
	}
	added := filing.links[len(links):]
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
