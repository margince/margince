// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The dormancy ladder on the company page: the strip's reading and the
// follow-up suggestion a cold account raises for its owner. Both read the one
// ladder in kernel/dormancy, over the company's stored last_activity_at.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/dormancy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

var suggestGoneDormant = crmcontracts.Company360SuggestionKindGoneDormant

// dormancyInputs is what the ladder and its suggestion read.
type dormancyInputs struct {
	reading      dormancy.Reading
	ladder       dormancy.Ladder
	lastActivity *time.Time
	// followUp is the installation's switch for the suggestion.
	followUp bool
	// owner is who the follow-up is prepared for. Nil raises no suggestion:
	// automatic work never falls to somebody else. An owner who cannot be
	// handed a task, such as a departed or read-only seat, reads as nil.
	owner *ids.UUID
	// ownerHasTask is an open task on the company already held by the owner,
	// such as the one an earlier press of the suggestion filed.
	ownerHasTask bool
}

func readDormancyInputs(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, heading companyHeading, now time.Time, loc *time.Location,
) (dormancyInputs, error) {
	policy, err := contacts.DormancyPolicyTx(ctx, tx)
	if err != nil {
		return dormancyInputs{}, err
	}
	out := dormancyInputs{
		reading:      policy.Ladder.Read(heading.lastActivity, now, loc),
		ladder:       policy.Ladder,
		lastActivity: heading.lastActivity,
		followUp:     policy.FollowUp,
	}
	if heading.owner == nil || !policy.FollowUp {
		return out, nil
	}
	eligible, err := auth.EligibleAssignees(ctx, tx, []ids.UUID{*heading.owner})
	if err != nil {
		return dormancyInputs{}, fmt.Errorf("read whether the account's owner can take a task: %w", err)
	}
	if !eligible[*heading.owner] {
		return out, nil
	}
	out.owner = heading.owner
	out.ownerHasTask, err = ownerHoldsOpenTask(ctx, tx, companyID, *heading.owner, now)
	return out, err
}

// ownerHoldsOpenTask answers whether the owner already holds an open task
// filed on the company. It asks about the owner's queue, not the reader's, so
// it takes no reader scope; it decides only whether a duplicate is offered.
func ownerHoldsOpenTask(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, owner ids.UUID, now time.Time,
) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos, ownerPos := arg(companyID), arg(owner)
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return false, err
	}
	var held bool
	err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS (
		SELECT 1 FROM activity a
		 WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND a.assignee_id = $%d
		   AND %s AND %s)`, ownerPos, activities.CompanyLinkedActivityExists(companyPos),
		activities.CurrentTaskClause("a", now, identities, arg)), args...).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("read whether the account's owner already holds a task on it: %w", err)
	}
	return held, nil
}

// quietSince reports whether the account is on a cold rung. It also needs
// newest, the newest message the reader sees, to be the ladder's dormant day old. The
// stored clock leaves out mail only its participants see, so a reader shown
// such mail yesterday is never told the account is dormant.
func (d dormancyInputs) quietSince(newest lastMessage, now time.Time, loc *time.Location) bool {
	if d.lastActivity == nil || !d.reading.Rung.Cold() {
		return false
	}
	return occurredday.DaysSince(occurredday.Moment{At: newest.At, DateOnly: newest.DateOnly}, now, loc) >= d.ladder.Dormant
}

// wire is the strip's reading, with the rule beside it.
func (d dormancyInputs) wire() *crmcontracts.DormancyReading {
	return &crmcontracts.DormancyReading{
		Rung:           crmcontracts.DormancyRung(d.reading.Rung),
		LastActivityAt: d.lastActivity,
		SilentDays:     d.reading.SilentDays,
		NoActivity:     d.reading.NoActivity,
		LadderDays:     d.ladder.Days(),
	}
}

// goneDormantSuggestion prepares a follow-up task for the owner of an account
// on the dormant or stale rung. An account that never had activity entered no
// rung, so it raises nothing. Nor does one whose owner already holds an open
// task on it, so pressing the suggestion files one task.
//
// The fingerprint is the silent stretch: the stored clock it is counted from.
// A dismissal holds for that stretch, and new activity starts a new one.
func goneDormantSuggestion(companyID ids.CompanyID, in suggestionInputs) *crmcontracts.Company360Suggestion {
	d := in.dormancy
	if !d.followUp || d.owner == nil || d.ownerHasTask || d.lastActivity == nil || !d.reading.Rung.Cold() {
		return nil
	}
	said := suggestionCopyFor(in.language)
	evidence := []crmcontracts.CompanyBriefEvidence{{
		EntityType: crmcontracts.CompanyBriefEvidenceEntityTypeCompany,
		EntityId:   openapi_types.UUID(companyID.UUID),
		At:         new(*d.lastActivity),
		Origin:     new(said.Say(suggestionWords.OriginLastActivity)),
	}}
	subject := said.Say(suggestionWords.DormantAccount)
	if in.companyName != "" {
		subject = fmt.Sprintf(said.Say(suggestionWords.DormantNamedAccount), in.companyName)
	}
	step := nextStep{body: TaskBody(subject, "company", companyID.UUID)}
	owner := openapi_types.UUID(*d.owner)
	step.body.AssigneeId = &owner
	out := &crmcontracts.Company360Suggestion{
		Kind:        suggestGoneDormant,
		Reason:      fmt.Sprintf(said.Say(suggestionWords.DormantReason), d.reading.SilentDays),
		Fingerprint: fingerprint(string(suggestGoneDormant), d.lastActivity.UTC().Format(time.RFC3339Nano), evidence),
		Evidence:    evidence,
		Title:       new(subject),
		DueAt:       new(*d.lastActivity),
	}
	setAddTask(out, step)
	return out
}
