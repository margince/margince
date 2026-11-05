// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/feedback"
	"github.com/margince/margince/backend/internal/shared/ports/introduction"
	"github.com/margince/margince/backend/internal/shared/ports/recordfiles"
)

// MergeSurvivorship collects authorized effects and record versions for one merge decision.
type MergeSurvivorship struct {
	CompanySingletons        []MergeCompanySingleton
	CompanyRelationshipTypes []string
	CompanyFacts             []MergeCompanyFact
	CompanyProfiles          []MergeCompanyProfile
	Signals                  []crmcontracts.Signal
	Claims                   []crmcontracts.ConversationClaim
	Handoffs                 []SDRHandoff
	Commitments              []crmcontracts.WeeklyPlanCommitment
	Notices                  []crmcontracts.NotificationItem
	Files                    []recordfiles.CarriedFile
	Draft                    *MergeDraft
	ProviderRuns             []MergeProviderRun
	Feedback                 []feedback.CarriedDecision
	ProfileFields            []MergeProfileField
	ChannelIdentities        []MergeChannelIdentity
	Social                   []MergeSocial
	Participants             []MergeParticipant
	Introductions            []introduction.CarriedRequest
	Fields                   map[string]any
	Grants                   []authz.CarriedRecordGrant
	Links                    []MergeLink
	Addresses                []MergeAddress
	Relationships            []MergeRelationship
	Observations             []MergeObservation
	EmploymentSupport        []MergeEmploymentSupport
	Consent                  ConsentCarryPreview
	Stops                    []commsauthz.CarriedStop
	Overrides                []commsauthz.CarriedOverride
	ConsentLinks             commsauthz.CarriedConsentLinks
	PreferenceLinks          []commsauthz.CarriedPreferenceLink
	SourceVersion            int64
	TargetVersion            int64
}

// PreviewMergeSurvivorship resolves authorized effects without applying the proposed change.
func (s *Store) PreviewMergeSurvivorship(ctx context.Context, kind string, source, target ids.UUID, sourceVersion, targetVersion *int64) (MergeSurvivorship, error) {
	var out MergeSurvivorship
	if kind != contactEntity && kind != companyEntity {
		return out, apperrors.ErrInvalidArgument
	}
	if source.IsZero() || target.IsZero() || source == target {
		return out, apperrors.ErrInvalidArgument
	}
	if err := requireDedupeRead(ctx, kind); err != nil {
		return out, err
	}
	if err := requireDedupeWrite(ctx, kind); err != nil {
		return out, err
	}
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		ctx, err := admitDuplicatePairMerge(ctx, tx, kind, source, target)
		if err != nil {
			return err
		}
		for _, id := range []ids.UUID{source, target} {
			if err := auth.EnsureWritableLive(ctx, tx, kind, id); err != nil {
				return err
			}
		}
		if err := storekit.EnsureIfMatch(ctx, tx, kind, source, sourceVersion); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, kind, target, targetVersion); err != nil {
			return err
		}
		out, err = readMergeSurvivorship(ctx, tx, kind, source, target)
		if err != nil {
			return err
		}
		return s.previewMergeScope(ctx, tx, kind, source, target, &out)
	})
	if err != nil {
		return MergeSurvivorship{}, err
	}
	return out, nil
}

func readMergeSurvivorship(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) (MergeSurvivorship, error) {
	if kind == contactEntity {
		src, tgt, err := mergePair(ctx, tx, kind, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), nil, readContactMergeState)
		if err != nil {
			return MergeSurvivorship{}, err
		}
		if src.Version == nil || tgt.Version == nil {
			return MergeSurvivorship{}, apperrors.ErrInvalidArgument
		}
		return MergeSurvivorship{Fields: buildSurvivorshipPatch(tgt, src).After(), SourceVersion: int64(*src.Version), TargetVersion: int64(*tgt.Version)}, nil
	}
	src, tgt, err := mergePair(ctx, tx, kind, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), nil, readCompanyMergeState)
	if err != nil {
		return MergeSurvivorship{}, err
	}
	if src.Version == nil || tgt.Version == nil {
		return MergeSurvivorship{}, apperrors.ErrInvalidArgument
	}
	if err := companyMergeAllowed(ctx, tx, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target)); err != nil {
		return MergeSurvivorship{}, err
	}
	return MergeSurvivorship{Fields: buildCompanySurvivorshipPatch(tgt, src).After(), SourceVersion: int64(*src.Version), TargetVersion: int64(*tgt.Version)}, nil
}

func companyMergeAllowed(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if err := refuseIfAnchor(ctx, tx, sourceID, "id", "it cannot be merged into another company. Archive the duplicate instead, and edit this one on the company page"); err != nil {
		return err
	}
	if err := refuseIfAnchor(ctx, tx, targetID, "target_id", "nothing can be merged into it. Archive the duplicate instead, and edit this one on the company page"); err != nil {
		return err
	}
	return refuseWhenBothCarryProjects(ctx, tx, sourceID, targetID)
}

func (s *Store) previewMergeScope(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID, out *MergeSurvivorship) error {
	var err error
	fields := slices.Collect(maps.Keys(out.Fields))
	for _, field := range fields {
		if strings.HasPrefix(field, "address_") {
			fields = append(fields, "address")
			break
		}
	}
	if err := previewFieldsVisible(ctx, kind, fields...); err != nil {
		return err
	}
	if err := s.previewMergeWork(ctx, tx, kind, source, out); err != nil {
		return err
	}
	if err := s.previewMergeContent(ctx, tx, kind, source, target, out); err != nil {
		return err
	}
	out.Feedback, err = s.previewFeedbackCarry(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.Grants, err = s.previewGrantCarry(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.Links, err = previewMergeLinks(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.Addresses, err = previewMergeAddresses(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.Relationships, err = s.previewMergeRelationships(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.Observations, err = s.previewMergeObservations(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.EmploymentSupport, err = s.previewMergeEmploymentSupport(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.Handoffs, err = previewMergeHandoffs(ctx, tx, kind, source)
	if err != nil {
		return err
	}
	if kind == contactEntity {
		return s.previewContactMergeScope(ctx, tx, source, target, out)
	}
	return previewCompanyMergeEvidence(ctx, tx, source, target, out)
}

func (s *Store) previewContactMergeScope(ctx context.Context, tx pgx.Tx, source, target ids.UUID, out *MergeSurvivorship) error {
	var err error
	out.Claims, err = s.previewMergeClaims(ctx, tx, source)
	if err != nil {
		return err
	}
	out.ProviderRuns, err = s.previewMergeProviderRuns(ctx, tx, source, target)
	if err != nil {
		return err
	}
	out.ProfileFields, err = s.previewMergeProfileFields(ctx, tx, source, target)
	if err != nil {
		return err
	}
	out.ChannelIdentities, err = previewMergeChannelIdentities(ctx, tx, source)
	if err != nil {
		return err
	}
	out.Social, err = previewMergeSocial(ctx, tx, source, target)
	if err != nil {
		return err
	}
	out.Participants, err = s.previewMergeParticipants(ctx, tx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target))
	if err != nil {
		return err
	}
	out.Introductions, err = s.previewIntroCarry(ctx, tx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target))
	if err != nil {
		return err
	}
	out.Consent, err = previewConsentCarry(ctx, tx, consentCarryContactMerge, source, target)
	if err != nil {
		return err
	}
	from, to := commsauthz.ContactStopSubject(ids.From[ids.ContactKind](source)), commsauthz.ContactStopSubject(ids.From[ids.ContactKind](target))
	out.Stops, err = s.previewCarriedStops(ctx, tx, from, to)
	if err != nil {
		return err
	}
	out.Overrides, err = s.previewCarriedOverrides(ctx, tx, from, to)
	if err != nil {
		return err
	}
	out.ConsentLinks, err = s.previewCarriedConsentLinks(ctx, tx, from, to)
	if err != nil {
		return err
	}
	out.PreferenceLinks, err = s.previewMergePreferenceLinks(ctx, tx, source, target, out.Addresses)
	return err
}

func (s *Store) previewMergeWork(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID, out *MergeSurvivorship) error {
	var err error
	out.Commitments, err = s.previewMergeCommitments(ctx, tx, kind, source)
	if err != nil {
		return err
	}
	out.Notices, err = s.previewMergeNotices(ctx, tx, kind, source)
	if err != nil {
		return err
	}
	out.Signals, err = s.previewMergeSignals(ctx, tx, kind, source)
	if err != nil {
		return err
	}
	return nil
}
