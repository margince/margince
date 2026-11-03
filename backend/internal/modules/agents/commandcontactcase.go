// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A contact case merge is merge_records over a reviewed group of contacts. It
// rides that verb so its tier and every installation tightening of it hold for
// the group as for a pair.

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ContactCaseMerger is the contacts module's case decision, the engine the
// review screen posts to.
type ContactCaseMerger interface {
	PreviewContactCaseMerge(ctx context.Context, cmd ContactCaseCommand) (crmcontracts.ContactCaseOutcome, error)
	MergeContactCase(ctx context.Context, cmd ContactCaseCommand) (crmcontracts.ContactCaseOutcome, error)
}

// WithContactCaseMerger binds the case decision merge_records reaches.
func WithContactCaseMerger(merger ContactCaseMerger) RegistryOption {
	return func(r *Registry) { r.contactCases = merger }
}

// ContactCaseCommand is one case decision, whichever door asked for it.
// ContactID addresses the case; the survivor is what remains.
type ContactCaseCommand struct {
	ContactID  ids.UUID
	Revision   string
	MemberIDs  []ids.UUID
	SurvivorID ids.UUID
	OwnerID    ids.UUID
	Preview    bool
}

// errCaseNeedsTwo refuses a group a merge could not shrink.
var errCaseNeedsTwo = errors.New("member_ids must name at least two contacts, survivor_id among them")

// NewContactCaseCall binds one case decision to the resolver that answers for
// it, reading the survivor through the record seam.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here must not leave this package
func NewContactCaseCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd ContactCaseCommand) GovernedCall {
	return bind[ContactCaseCommand](&contactCaseResolver{
		language: language,
		survivor: anchoredRecord{records: records, entityType: datasource.EntityContact},
	}, cmd)
}

type contactCaseResolver struct {
	survivor anchoredRecord
	language baselanguage.Resolver
}

// Subject pins the survivor, the record every member folds into.
func (r *contactCaseResolver) Subject(ctx context.Context, cmd ContactCaseCommand) (StageInfo, error) {
	if err := requireCaseSelection(cmd); err != nil {
		return StageInfo{}, err
	}
	rec, err := r.survivor.row(ctx, cmd.SurvivorID)
	if err != nil {
		return StageInfo{}, err
	}
	said := summaryIn(ctx, r.language)
	sentence := said.mergeCase
	if cmd.Preview {
		sentence = said.previewCase
	}
	return StageInfo{
		TargetType:    string(datasource.EntityContact),
		TargetID:      cmd.SurvivorID,
		TargetVersion: &rec.Version,
		Summary:       fmt.Sprintf(sentence, len(cmd.MemberIDs), recordLabel(rec)),
	}, nil
}

// Guards refuses a group with nothing to merge and a survivor the caller
// cannot see or whose authority lives elsewhere. The case engine re-checks
// every member and the revision when the decision runs.
func (r *contactCaseResolver) Guards(ctx context.Context, cmd ContactCaseCommand) error {
	if err := requireCaseSelection(cmd); err != nil {
		return err
	}
	return r.survivor.refuse(ctx, cmd.SurvivorID)
}

func requireCaseSelection(cmd ContactCaseCommand) error {
	survivorListed := false
	for _, id := range cmd.MemberIDs {
		survivorListed = survivorListed || id == cmd.SurvivorID
	}
	if len(cmd.MemberIDs) < 2 || !survivorListed {
		return &BadArgsError{Cause: errCaseNeedsTwo}
	}
	return nil
}
