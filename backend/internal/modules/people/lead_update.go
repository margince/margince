// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type UpdateLeadInput struct {
	// Clear names the wire fields to set to NULL. A JSON null cannot say so —
	// it decodes to a nil pointer and reads as "not supplied" — so the
	// reversal path names them here instead.
	Clear []string
	// Trail names what the audit trail calls this write; zero is an update.
	Trail           storekit.AuditTrail
	FullName        *string
	Email           *string
	Title           *string
	CompanyName     *string
	CandidateOrgKey *string
	Status          *string // only new ↔ working here; terminal states have their own paths
	// Source corrects where the lead came from; the score follows it.
	Source *string
	Score  *int
	// ScoreOverrideReason is the written reason for a score override; nil
	// means the field was absent (no override change). The explicit CLEAR
	// gesture is ClearScoreOverride, not an empty string — an empty reason
	// is invalid input (contract minLength 1).
	ScoreOverrideReason *string
	// ClearScoreOverride is the wire's explicit JSON null on score or
	// score_override_reason: drop the override and resume recompute.
	// encoding/json erases null-vs-absent on pointer fields, so the
	// transports carry the distinction here.
	ClearScoreOverride bool
	OwnerID            *ids.UserID
	ProjectID          *ids.ProjectID
	IfVersion          *int64
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

// ScoreOverrideReasonRequiredError rejects a human score with no written
// reason — the Commercial Judgement rule (formulas §3.1, AC-S1): an
// override is auditable or it does not happen.
type ScoreOverrideReasonRequiredError struct{}

func (e *ScoreOverrideReasonRequiredError) Error() string {
	return "a score override requires a non-empty score_override_reason"
}

// FieldFault refuses a score override with no stated reason.
func (e *ScoreOverrideReasonRequiredError) FieldFault() (field, code, message string) {
	return "score_override_reason", codeRequired, e.Error()
}

// leadScoreField names the lead's own score input. Its own constant, not the
// dedupe engine's evidenceScoreKey: those two spell the same word for unrelated
// reasons, and borrowing one for the other ties this wire contract to a change
// made for a different feature.
const leadScoreField = "score"

// ScoreOverrideWithoutScoreError is the mirror of
// ScoreOverrideReasonRequiredError: a reason arrived with no score to attach it
// to, so the missing input is the SCORE. Its own type because the two
// conditions name different fields, and one error for both would tell half the
// callers to fix an input they had already supplied.
type ScoreOverrideWithoutScoreError struct{}

func (e *ScoreOverrideWithoutScoreError) Error() string {
	return "a score_override_reason without a score sets nothing; send the score too"
}

// FieldFault names the score, which is the input that is missing.
func (e *ScoreOverrideWithoutScoreError) FieldFault() (field, code, message string) {
	return leadScoreField, codeRequired, e.Error()
}

// ScoreOverrideReasonEmptyError rejects an empty-string reason: the
// contract's clear gesture is JSON null (minLength 1 on the field), so a
// blank reason is neither a written justification nor a clear — it is
// invalid input.
type ScoreOverrideReasonEmptyError struct{}

func (e *ScoreOverrideReasonEmptyError) Error() string {
	return "score_override_reason must not be empty; pass null to clear the override"
}

// FieldFault refuses a score-override reason that is present but blank.
func (e *ScoreOverrideReasonEmptyError) FieldFault() (field, code, message string) {
	return "score_override_reason", "min_length", e.Error()
}

// ScoreOverrideClearConflictError rejects a null score arriving together
// with a written reason: null says "drop the override", the reason says
// "keep one" — honoring either would silently discard the other half of
// the request.
type ScoreOverrideClearConflictError struct{}

func (e *ScoreOverrideClearConflictError) Error() string {
	return "a null score clears the override; it cannot be combined with a score_override_reason"
}

// FieldFault refuses setting and clearing the same override in one request.
func (e *ScoreOverrideClearConflictError) FieldFault() (field, code, message string) {
	return leadScoreField, "clear_conflict", e.Error()
}

func (s *Store) UpdateLead(ctx context.Context, id ids.LeadID, in UpdateLeadInput) (crmcontracts.Lead, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return crmcontracts.Lead{}, err
	}
	active, err := s.activeColumns(ctx, "lead")
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	var out crmcontracts.Lead
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.updateLeadTx(ctx, tx, id, in, active)
		return err
	})
	return out, err
}

// CapturedLeadFields is what an inbound message knew about a lead that already
// exists. Every field is optional in practice: a provider payload carries what
// it carries.
type CapturedLeadFields struct {
	FullName    string
	Email       string
	CompanyName string
	Title       string
}

// FillEmptyLeadFieldsTx folds captured values onto a lead, filling ONLY the
// fields the lead does not already have.
//
// A captured value never overwrites one already there. The incumbent's value
// may have been typed by a person, and an inbound message carries no evidence
// that it knows better — the same rule the enrich and cold-start accepts keep,
// and what makes accepting the card safe enough to offer at all.
//
// The comparison happens HERE, under a row lock taken inside the caller's
// transaction, rather than in the seam that calls it. A caller reading the lead
// first and patching second would leave a window in which a person's edit lands
// between the two and is then overwritten by a value this rule exists to keep
// out.
//
// Writing nothing is a normal outcome, not a failure: a lead that already
// carries everything the message knew is a lead the fold has no work on.
//
// `active` is the workspace's custom-field catalog, fetched by the caller
// BEFORE its transaction opened. Reading it here would open a second connection
// inside somebody else's transaction — it commits separately and can deadlock
// undetectably against a lock that transaction holds.
func (s *Store) FillEmptyLeadFieldsTx(ctx context.Context, tx pgx.Tx, id ids.LeadID,
	in CapturedLeadFields, active CustomColumns,
) error {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return err
	}
	// The row-scope gate comes BEFORE the read, not with the write.
	//
	// `readLead` is a bare `WHERE id = $1` that trusts its caller to have gated
	// already — so reading first and leaving the gate to the write would
	// disclose a lead's field values to a caller outside its scope, and
	// disclose them most completely in the case that writes nothing.
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		return err
	}
	// And the LOCK comes before the decision read, which is the rule for every
	// internal multi-step flow (storekit.ApplyGuarded's own comment states it).
	// Deciding which fields are empty from an unlocked read leaves an interval
	// in which a person fills one; the write would then wait for its lock and
	// overwrite what they typed with a value this rule exists to keep out.
	if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	current, err := readLead(ctx, tx, id, storekit.LiveOnly, active.cols)
	if err != nil {
		return err
	}
	patch := emptyFieldPatch(current, in)
	if patch.FullName == nil && patch.CompanyName == nil && patch.Title == nil {
		return nil
	}
	_, err = s.updateLeadTx(ctx, tx, id, patch, active.cols)
	return err
}

// emptyFieldPatch is the sparse patch: a captured value for every field the
// lead lacks, and nothing for the rest.
//
// Email is deliberately absent. It is the lead's dedupe key and the reason this
// collision was detected at all, so the incumbent necessarily has one — a
// captured address could only ever be a second address, which is an employment
// question rather than a blank to fill.
func emptyFieldPatch(current crmcontracts.Lead, in CapturedLeadFields) UpdateLeadInput {
	var patch UpdateLeadInput
	if in.FullName != "" && blank(current.FullName) {
		patch.FullName = &in.FullName
	}
	if in.CompanyName != "" && blank(current.CompanyName) {
		patch.CompanyName = &in.CompanyName
	}
	if in.Title != "" && blank(current.Title) {
		patch.Title = &in.Title
	}
	return patch
}

func blank(value *string) bool { return value == nil || *value == "" }

// updateLeadTx runs the visibility gate, the sparse-patch fold, the write
// shape, and the cleared-override recompute for one lead update inside the
// caller's transaction. active names the workspace's custom-field columns
// (fetched before the tx opened).
func (s *Store) updateLeadTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, in UpdateLeadInput, active []fieldcatalog.Column) (crmcontracts.Lead, error) {
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		return crmcontracts.Lead{}, err
	}
	current, err := readLead(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	// A client-supplied reference to a row-scoped record is a read of it.
	// Deliberately no same-company check: a lead has no company to compare
	// (Note PROJ-DDL-N-4).
	if in.ProjectID != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, "project", in.ProjectID.UUID); err != nil {
			return crmcontracts.Lead{}, err
		}
	}
	if in.Source != nil {
		if err := ensureHumanSourceAllowed(ctx, tx, strings.TrimSpace(*in.Source)); err != nil {
			return crmcontracts.Lead{}, err
		}
	}
	p, resumeRecompute, err := buildLeadPatch(current, in)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := stampHumanFirstResponse(ctx, p, current, in); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := stampStatusSetBy(ctx, p, current, in); err != nil {
		return crmcontracts.Lead{}, err
	}
	storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties)
	if p.Empty() {
		return current, nil
	}
	if err := p.ApplyGuarded(ctx, tx, "lead", id.UUID, in.IfVersion); err != nil {
		if mapped, ok := leadUniqueViolation(err, in.Email); ok {
			return crmcontracts.Lead{}, mapped
		}
		return crmcontracts.Lead{}, err
	}
	auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "lead", id.UUID, p.Before(), p.After())
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadUpdated{ChangedFields: p.After()}); err != nil {
		return crmcontracts.Lead{}, err
	}
	// Clearing an override immediately recomputes from current signals
	// (formulas §3.1): score no longer lags behind the machine value, and
	// the recompute appends its own history entry.
	// A rename or a new company/address can make this lead read like another
	// one; the review trail follows the identity, not just the create.
	if identityTouched(p, leadNameColumn, leadCompanyColumn, leadEmailColumn) {
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return crmcontracts.Lead{}, err
		}
		updated, err := readLead(ctx, tx, id, storekit.LiveOnly, nil)
		if err != nil {
			return crmcontracts.Lead{}, err
		}
		if err := s.recordLeadNearMatch(ctx, tx, updated, by); err != nil {
			return crmcontracts.Lead{}, err
		}
	}
	if resumeRecompute {
		if err := recomputeLeadScoreTx(ctx, tx, id, time.Now().UTC(), true); err != nil {
			return crmcontracts.Lead{}, err
		}
	} else if in.Source != nil && *in.Source != current.Source {
		// The source weight is part of the score (formulas §3.1): a corrected
		// source is rescored at once. Under an override the recompute moves
		// score_computed and leaves the human's number alone.
		if err := recomputeLeadScoreTx(ctx, tx, id, time.Now().UTC(), false); err != nil {
			return crmcontracts.Lead{}, err
		}
	} else if in.Score != nil {
		// SETTING an override moves the displayed score without touching the
		// machine value, so no recompute runs and nothing else records it.
		// Without this entry the newest point in the series still holds the
		// pre-override number, and "Explain This Score" would answer for a
		// score the lead no longer carries (ADR-0105 §5).
		if err := appendOverrideScoreHistory(ctx, tx, id); err != nil {
			return crmcontracts.Lead{}, err
		}
	}
	return readLead(ctx, tx, id, storekit.LiveOnly, active)
}

// buildLeadPatch folds the caller's sparse update onto the current lead
// as a field patch, and reports whether the caller must resume recompute
// (a cleared score override). The Commercial Judgement score override
// (formulas §3.1, A68/ADR-0053) is sticky: setting a score demands a
// written reason and retains the machine value; clearing the reason
// resumes recompute.
func buildLeadPatch(current crmcontracts.Lead, in UpdateLeadInput) (*storekit.Patch, bool, error) {
	p := storekit.NewPatch()
	if err := storekit.ApplyClears(p, in.Clear, clearableLeadColumns(current)); err != nil {
		return nil, false, err
	}
	if in.FullName != nil {
		p.Set("full_name", current.FullName, *in.FullName)
	}
	if in.Email != nil {
		parsed, err := values.ParseEmail(*in.Email)
		if err != nil {
			return nil, false, err
		}
		p.Set("email", current.Email, parsed.String())
	}
	if in.Title != nil {
		p.Set("title", current.Title, *in.Title)
	}
	if in.CompanyName != nil {
		p.Set("company_name", current.CompanyName, *in.CompanyName)
	}
	if in.CandidateOrgKey != nil {
		p.Set("candidate_org_key", current.CandidateOrgKey, *in.CandidateOrgKey)
	}
	if in.ProjectID != nil {
		p.Set("project_id", current.ProjectId, *in.ProjectID)
	}
	if in.Status != nil {
		status, err := parseWritableLeadStatus(*in.Status)
		if err != nil {
			return nil, false, err
		}
		p.Set(leadStatusColumn, current.Status, string(status))
	}
	if in.Source != nil {
		if strings.TrimSpace(*in.Source) == "" {
			return nil, false, &values.ParseError{Field: "source", Code: codeRequired, Message: "source must not be empty"}
		}
		p.Set("source", current.Source, strings.TrimSpace(*in.Source))
	}
	resumeRecompute, err := applyScoreOverride(p, current, in)
	if err != nil {
		return nil, false, err
	}
	if in.OwnerID != nil {
		p.Set("owner_id", current.OwnerId, *in.OwnerID)
	}
	return p, resumeRecompute, nil
}

// applyScoreOverride folds the §3.1 sticky-override rules into the patch
// and reports whether the caller must resume recompute (an override was
// cleared). Setting `score` establishes/refreshes an override — it
// requires a non-empty reason and captures the machine value into
// score_computed the first time. An explicit JSON null on score or the
// reason clears the override. A non-empty reason with no score amends
// the note on an override already in force; an empty-string reason is
// invalid input (the clear gesture is null, not "").
func applyScoreOverride(p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) (resumeRecompute bool, err error) {
	overrideInForce := current.ScoreOverrideReason != nil

	switch {
	case in.Score != nil:
		reason := ""
		if in.ScoreOverrideReason != nil {
			reason = strings.TrimSpace(*in.ScoreOverrideReason)
		}
		if reason == "" {
			return false, &ScoreOverrideReasonRequiredError{}
		}
		p.Set("score", current.Score, *in.Score)
		p.Set("score_override_reason", current.ScoreOverrideReason, reason)
		// Retain the last machine value the first time an override takes
		// hold; if one is already in force, score_computed already holds it
		// and the recompute keeps it fresh — don't clobber it with a human
		// number.
		if !overrideInForce {
			p.Set("score_computed", current.ScoreComputed, current.Score)
		}
		return false, nil

	case in.ClearScoreOverride:
		if in.ScoreOverrideReason != nil {
			return false, &ScoreOverrideClearConflictError{}
		}
		if !overrideInForce {
			return false, nil // no override to clear — a no-op
		}
		p.Set("score_override_reason", current.ScoreOverrideReason, nil)
		// Resume: score tracks the retained machine value, then recompute
		// refines it from current signals.
		if current.ScoreComputed != nil {
			p.Set("score", current.Score, *current.ScoreComputed)
		}
		p.Set("score_computed", current.ScoreComputed, nil)
		return true, nil

	case in.ScoreOverrideReason != nil:
		if strings.TrimSpace(*in.ScoreOverrideReason) == "" {
			return false, &ScoreOverrideReasonEmptyError{}
		}
		if !overrideInForce {
			return false, &ScoreOverrideWithoutScoreError{}
		}
		p.Set("score_override_reason", current.ScoreOverrideReason, strings.TrimSpace(*in.ScoreOverrideReason))
		return false, nil
	}
	return false, nil
}

// stampHumanFirstResponse adds the §18.1 first-response stamp to a patch
// that moves the lead off `new` by a HUMAN's hand. An agent's status change
// is not a response, and a lead already answered keeps its first stamp.
func stampHumanFirstResponse(ctx context.Context, p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) error {
	if in.Status == nil || current.Status != crmcontracts.LeadStatusNew || current.FirstResponseAt != nil {
		return nil
	}
	if LeadStatus(*in.Status) == LeadStatusNew {
		return nil
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	if actor.Type == principal.PrincipalHuman {
		p.Set(firstResponseColumn, nil, time.Now().UTC())
	}
	return nil
}

// leadStatusSetByColumn records who placed the lead on its current step.
const leadStatusSetByColumn = "status_set_by"

// stampStatusSetBy records that a status written through this path was a
// hand's doing — a human, or an agent acting for one — as opposed to the
// system's own climb from captured activity (advanceLeadStatusTx).
func stampStatusSetBy(ctx context.Context, p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) error {
	if in.Status == nil || LeadStatus(*in.Status) == LeadStatus(current.Status) {
		return nil
	}
	setBy, err := statusSetByFor(ctx)
	if err != nil {
		return err
	}
	p.Set(leadStatusSetByColumn, current.StatusSetBy, setBy)
	return nil
}

// statusSetByFor names who is placing the lead on its status: the system
// when the actor is the system principal (a workflow, a migration-time
// repair), a human otherwise — an agent acting for a human counts as the
// human's hand, because the human's seat is what admitted it.
func statusSetByFor(ctx context.Context) (string, error) {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return "", err
	}
	if actor.Type == principal.PrincipalSystem {
		return string(crmcontracts.LeadStatusSetBySystem), nil
	}
	return string(crmcontracts.LeadStatusSetByHuman), nil
}
