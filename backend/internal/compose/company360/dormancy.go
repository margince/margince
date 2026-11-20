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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/dormancy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	// automatic work never falls to somebody else.
	owner *ids.UUID
}

func readDormancyInputs(
	ctx context.Context, tx pgx.Tx, heading companyHeading, now time.Time, loc *time.Location,
) (dormancyInputs, error) {
	policy, err := contacts.DormancyPolicyTx(ctx, tx)
	if err != nil {
		return dormancyInputs{}, err
	}
	return dormancyInputs{
		reading:      policy.Ladder.Read(heading.lastActivity, now, loc),
		ladder:       policy.Ladder,
		lastActivity: heading.lastActivity,
		followUp:     policy.FollowUp,
		owner:        heading.owner,
	}, nil
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
// rung, so it raises nothing.
//
// The fingerprint is the silent stretch: the stored clock it is counted from.
// A dismissal holds for that stretch, and new activity starts a new one.
func goneDormantSuggestion(companyID ids.CompanyID, in suggestionInputs) *crmcontracts.Company360Suggestion {
	d := in.dormancy
	if !d.followUp || d.owner == nil || d.lastActivity == nil || !d.reading.Rung.Cold() {
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
