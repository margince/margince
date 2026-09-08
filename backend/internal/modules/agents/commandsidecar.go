// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The company-sidecar commands: a fact or a profile field is not a
// column on `company` itself but a row keyed by a SECOND path segment
// (factKey, field) naming WHICH sidecar row the call is about. The approval
// still binds to the company — that is the row whose visibility and
// system-of-record the human's decision actually depends on — but the
// operand has to travel with the command, or two different facts on the same
// company become one indistinguishable call the moment either is
// staged (margince/margince#928 task 5: "their operand lives in the
// URL path").

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// companySidecarRecordType is every command in this file's fixed
// target: the company the fact or profile field belongs to. There is
// nothing on the record seam a fact or profile field row could be pointed at
// on its own.
const companySidecarRecordType = string(datasource.EntityCompany)

// ConfirmFactCommand is one company fact confirmation, whichever door
// asked for it. It carries no body: PO-AC-N-3 confirmation changes no value,
// only who last agreed with the machine's extraction.
type ConfirmFactCommand struct {
	ID      ids.UUID
	FactKey string
}

// NewConfirmFactCall binds one fact confirmation to the resolver that
// answers for it, reading through the record seam the company itself
// writes through.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewConfirmFactCall(records datasource.SystemOfRecordProvider, cmd ConfirmFactCommand) GovernedCall {
	return bind[ConfirmFactCommand](confirmFactResolver{
		target: routedRecordTarget{records: records, recordType: companySidecarRecordType},
	}, cmd)
}

type confirmFactResolver struct {
	target routedRecordTarget
}

// Subject names the COMPANY the approval binds to — a fact has no row
// of its own on the seam — with the fact key carried into the summary: the
// door-agnostic line GovernedCall.Subject owes this operation, distinct per
// fact even though no door renders it today (REST takes its own line from
// restSummary; no tool reaches this command at all — agentgatestaging.go).
// It reads nothing: unlike archiveResolver's, there is no per-record label
// to compose (routedRecordTarget's own doc says why).
func (r confirmFactResolver) Subject(_ context.Context, cmd ConfirmFactCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: companySidecarRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf("Confirm fact %s on company %s", cmd.FactKey, cmd.ID),
	}, nil
}

// Guards refuses, before anything is staged, a company the caller
// cannot see or whose authority lives elsewhere — the same two refusals
// patchResolver.Guards makes for its own target. It does NOT check whether
// FactKey names an existing fact: that read is the handler's, not this
// approval's, and restating it here would be a second copy of the
// executor's own rule.
func (r confirmFactResolver) Guards(ctx context.Context, cmd ConfirmFactCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// UpdateFactCommand is one company fact correction, whichever door
// asked for it — the routed company id and the fact key being
// corrected. It does NOT carry the corrected value: neither Guards nor
// Subject reads one (the body travels separately, into diff_hash, and
// nothing here renders it — see Subject's own doc), so a field with no
// reader is not carried, the same call task 4's review made for
// PatchCommand.IfVersion.
type UpdateFactCommand struct {
	ID      ids.UUID
	FactKey string
}

// NewUpdateFactCall binds one fact correction to the resolver that answers
// for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewUpdateFactCall(records datasource.SystemOfRecordProvider, cmd UpdateFactCommand) GovernedCall {
	return bind[UpdateFactCommand](updateFactResolver{
		target: routedRecordTarget{records: records, recordType: companySidecarRecordType},
	}, cmd)
}

type updateFactResolver struct {
	target routedRecordTarget
}

// Subject, Guards: the same shape as confirmFactResolver's.
func (r updateFactResolver) Subject(_ context.Context, cmd UpdateFactCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: companySidecarRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf("Update fact %s on company %s", cmd.FactKey, cmd.ID),
	}, nil
}

func (r updateFactResolver) Guards(ctx context.Context, cmd UpdateFactCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// CreateFactCommand is one hand-stated company fact. It carries only the
// routed company id: what makes a fact is its category, field and value,
// and all three travel in the body into diff_hash rather than being rendered
// by Subject — the same call UpdateFactCommand makes about the corrected value
// it deliberately does not carry.
type CreateFactCommand struct {
	ID ids.UUID
}

// NewCreateFactCall binds one hand-stated fact to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewCreateFactCall(records datasource.SystemOfRecordProvider, cmd CreateFactCommand) GovernedCall {
	return bind[CreateFactCommand](createFactResolver{
		target: routedRecordTarget{records: records, recordType: companySidecarRecordType},
	}, cmd)
}

type createFactResolver struct {
	target routedRecordTarget
}

func (r createFactResolver) Subject(_ context.Context, cmd CreateFactCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: companySidecarRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf("State a fact on company %s", cmd.ID),
	}, nil
}

func (r createFactResolver) Guards(ctx context.Context, cmd CreateFactCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// DeleteFactCommand is one company fact removal — the routed company
// id and the fact key being removed, the same two operands a correction needs,
// because which row is removed is the whole of what an approval binds to.
type DeleteFactCommand struct {
	ID      ids.UUID
	FactKey string
}

// NewDeleteFactCall binds one fact removal to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewDeleteFactCall(records datasource.SystemOfRecordProvider, cmd DeleteFactCommand) GovernedCall {
	return bind[DeleteFactCommand](deleteFactResolver{
		target: routedRecordTarget{records: records, recordType: companySidecarRecordType},
	}, cmd)
}

type deleteFactResolver struct {
	target routedRecordTarget
}

func (r deleteFactResolver) Subject(_ context.Context, cmd DeleteFactCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: companySidecarRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf("Remove fact %s from company %s", cmd.FactKey, cmd.ID),
	}, nil
}

func (r deleteFactResolver) Guards(ctx context.Context, cmd DeleteFactCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// ConfirmProfileFieldCommand is one company profile-field confirmation,
// whichever door asked for it — Field is the closed-vocabulary profile-field
// key (`display_name`, `icp`, …), never a fact key.
type ConfirmProfileFieldCommand struct {
	ID    ids.UUID
	Field string
}

// NewConfirmProfileFieldCall binds one profile-field confirmation to the
// resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewConfirmProfileFieldCall(records datasource.SystemOfRecordProvider, cmd ConfirmProfileFieldCommand) GovernedCall {
	return bind[ConfirmProfileFieldCommand](confirmProfileFieldResolver{
		target: routedRecordTarget{records: records, recordType: companySidecarRecordType},
	}, cmd)
}

type confirmProfileFieldResolver struct {
	target routedRecordTarget
}

// Subject, Guards: the same shape as confirmFactResolver's, naming Field
// instead of FactKey as the summary's operand.
func (r confirmProfileFieldResolver) Subject(_ context.Context, cmd ConfirmProfileFieldCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: companySidecarRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf("Confirm profile field %s on company %s", cmd.Field, cmd.ID),
	}, nil
}

func (r confirmProfileFieldResolver) Guards(ctx context.Context, cmd ConfirmProfileFieldCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}

// UpdateProfileFieldCommand is one company profile-field correction,
// whichever door asked for it — the routed company id and the field
// being corrected. It does not carry the corrected value, the same reason
// UpdateFactCommand's own doc gives.
type UpdateProfileFieldCommand struct {
	ID    ids.UUID
	Field string
}

// NewUpdateProfileFieldCall binds one profile-field correction to the
// resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewUpdateProfileFieldCall(records datasource.SystemOfRecordProvider, cmd UpdateProfileFieldCommand) GovernedCall {
	return bind[UpdateProfileFieldCommand](updateProfileFieldResolver{
		target: routedRecordTarget{records: records, recordType: companySidecarRecordType},
	}, cmd)
}

type updateProfileFieldResolver struct {
	target routedRecordTarget
}

func (r updateProfileFieldResolver) Subject(_ context.Context, cmd UpdateProfileFieldCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: companySidecarRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf("Update profile field %s on company %s", cmd.Field, cmd.ID),
	}, nil
}

func (r updateProfileFieldResolver) Guards(ctx context.Context, cmd UpdateProfileFieldCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}
