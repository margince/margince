// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Attaching or detaching a project stakeholder: commands the tool schema cannot
// express. Neither is a field patch, so neither belongs in patchResolver. Both
// target `project`, which the record seam serves, through routedRecordTarget.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	// projectRecordType names the fixed target of this file's stakeholder
	// commands.
	projectRecordType = string(datasource.EntityProject)
)

// SetStakeholderCommand is one project-stakeholder attach or re-role,
// whichever door asked for it — the routed project id only. It does not
// carry the attached contact or role: neither Guards nor Subject reads them
// (setStakeholderResolver.Subject's own doc says why), the same reason
// UpdateFactCommand's own doc (commandsidecar.go) gives for dropping a
// value nothing here reads.
type SetStakeholderCommand struct {
	ID ids.UUID
}

// NewSetStakeholderCall binds one attach/re-role to the resolver that
// answers for it, reading through the record seam the project itself
// writes through.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewSetStakeholderCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd SetStakeholderCommand) GovernedCall {
	return bind[SetStakeholderCommand](setStakeholderResolver{
		language: language,
		target:   routedRecordTarget{records: records, recordType: projectRecordType},
	}, cmd)
}

type setStakeholderResolver struct {
	target   routedRecordTarget
	language baselanguage.Resolver
}

// Subject names the PROJECT the approval binds to — a stakeholder edge has
// no row of its own on the seam. Unlike removeStakeholderResolver's, there
// is no path operand to carry into the summary: contact_id and role arrive in
// the BODY here, and the body's own fields are what the inbox shows beside
// this line (proposed_change), the same reasoning patchResolver's own
// Subject gives for not repeating a patch's values.
func (r setStakeholderResolver) Subject(ctx context.Context, cmd SetStakeholderCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: projectRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf(summaryIn(ctx, r.language).setStakeholder, cmd.ID),
	}, nil
}

// Guards refuses, before anything is staged, a project the caller cannot see
// or whose authority lives elsewhere — the same two refusals
// patchResolver.Guards makes for its own target. It does not check whether
// the named contact exists or is already a stakeholder: those reads are the
// handler's, not this approval's.
func (r setStakeholderResolver) Guards(ctx context.Context, cmd SetStakeholderCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// RemoveStakeholderCommand is one project-stakeholder detach, whichever door
// asked for it. ContactID is a second PATH parameter, not a body field — the
// operand this whole task exists to carry.
type RemoveStakeholderCommand struct {
	ID        ids.UUID
	ContactID ids.UUID
}

// NewRemoveStakeholderCall binds one detach to the resolver that answers
// for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewRemoveStakeholderCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd RemoveStakeholderCommand) GovernedCall {
	return bind[RemoveStakeholderCommand](removeStakeholderResolver{
		language: language,
		target:   routedRecordTarget{records: records, recordType: projectRecordType},
	}, cmd)
}

type removeStakeholderResolver struct {
	target   routedRecordTarget
	language baselanguage.Resolver
}

// Subject names the PROJECT the approval binds to, with the contact being
// detached carried into the summary: the door-agnostic line
// GovernedCall.Subject owes this operation, distinct per contact, even
// though no door renders it today (confirmFactResolver's own doc says why).
func (r removeStakeholderResolver) Subject(ctx context.Context, cmd RemoveStakeholderCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: projectRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf(summaryIn(ctx, r.language).removeStakeholder, cmd.ContactID, cmd.ID),
	}, nil
}

// Guards: the same two refusals as setStakeholderResolver's. It does not
// check whether ContactID is currently a stakeholder — the edge's own
// existence is the handler's rule, and this approval binds to the project
// regardless of whether the edge is still there when it is redeemed.
func (r removeStakeholderResolver) Guards(ctx context.Context, cmd RemoveStakeholderCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// SetCompanyCommand is one project-company attach or re-role, whichever door
// asked for it — the routed project id only. It does not carry the company or
// role: neither Guards nor Subject reads them, and the body's own fields are
// what the inbox shows beside the line, the same reasoning
// SetStakeholderCommand's own doc gives.
type SetCompanyCommand struct {
	ID ids.UUID
}

// NewSetCompanyCall binds one company attach/re-role to the resolver that
// answers for it, reading through the record seam the project writes through.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewSetCompanyCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd SetCompanyCommand) GovernedCall {
	return bind[SetCompanyCommand](setCompanyResolver{
		language: language,
		target:   routedRecordTarget{records: records, recordType: projectRecordType},
	}, cmd)
}

type setCompanyResolver struct {
	target   routedRecordTarget
	language baselanguage.Resolver
}

// Subject names the PROJECT the approval binds to — a company edge has no row
// of its own on the seam, exactly as a stakeholder edge has none.
func (r setCompanyResolver) Subject(ctx context.Context, cmd SetCompanyCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: projectRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf(summaryIn(ctx, r.language).setCompany, cmd.ID),
	}, nil
}

// Guards refuses, before anything is staged, a project the caller cannot see or
// whose authority lives elsewhere. It does not check whether the named company
// exists or is already on the project: those reads are the handler's.
func (r setCompanyResolver) Guards(ctx context.Context, cmd SetCompanyCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// RemoveCompanyCommand is one project-company detach. CompanyID is a
// second PATH parameter, not a body field.
type RemoveCompanyCommand struct {
	ID        ids.UUID
	CompanyID ids.UUID
}

// NewRemoveCompanyCall binds one detach to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewRemoveCompanyCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd RemoveCompanyCommand) GovernedCall {
	return bind[RemoveCompanyCommand](removeCompanyResolver{
		language: language,
		target:   routedRecordTarget{records: records, recordType: projectRecordType},
	}, cmd)
}

type removeCompanyResolver struct {
	target   routedRecordTarget
	language baselanguage.Resolver
}

// Subject names the PROJECT, with the company being taken off carried into the
// summary: the door-agnostic line this operation owes, distinct per company.
func (r removeCompanyResolver) Subject(ctx context.Context, cmd RemoveCompanyCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: projectRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf(summaryIn(ctx, r.language).removeCompany, cmd.CompanyID, cmd.ID),
	}, nil
}

// Guards: the same two refusals as setCompanyResolver's. It does not check
// whether the company is currently on the project — that is the handler's rule,
// including the refusal that keeps the last one there.
func (r removeCompanyResolver) Guards(ctx context.Context, cmd RemoveCompanyCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}
