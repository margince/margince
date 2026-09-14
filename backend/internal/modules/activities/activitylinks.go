// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The timeline's polymorphic link rows: what an activity is filed under, and
// the rules every writer of one meets — the vocabulary it may name, the bound
// on how many, and the row-scope check on each. Split from activity.go because
// a link is its own concept: a send, a booking, a logged note and a captured
// message all reach this and share little else.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivityLinkInput ties one activity to one record. Which record types are
// admitted is linkTargets' answer, not a list restated here.
type ActivityLinkInput struct {
	EntityType string // one of linkTargets (activity_link_entity_type_check)
	// note: the link target is polymorphic (activity_link is the canonical
	// (entity_type, entity_id) seam), so the id stays untyped (rule 6).
	EntityID ids.UUID
}

// fieldLinks is the request field every link refusal attributes itself to, so
// a caller reading a 422 is told which array to change. One spelling, because
// three different refusals name it and a fourth that spelled it differently
// would send the caller looking for a field the request does not have.
const fieldLinks = "links"

// maxActivityLinks bounds how many records one activity may be filed under.
//
// It is a REQUEST BOUND rather than a modelling opinion: every entry costs its
// own row-scoped probe and its own insert, and the array is chosen freely by
// the caller. Both request schemas that carry a link list declare the same 25,
// and the agent surface refuses the count earlier so no approval is minted for
// a call the store would reject — this is the bound that holds for every
// transport, a human's own send included. insertActivityLinks applies it at
// the write itself, which is where a booking meets it.
//
// Two sibling modules and the trigger that enforces it spell the same number.
// Held by: TestTheLinkCeilingHasOneValue (backend/gates/linkceilingparity_test.go)
const maxActivityLinks = 25

// TooManyLinksError refuses an activity filed under more records than the
// timeline will carry for one entry.
type TooManyLinksError struct{ Count int }

func (e *TooManyLinksError) Error() string {
	return fmt.Sprintf("an activity may be filed under at most %d records; this one names %d",
		maxActivityLinks, e.Count)
}

// FieldFault names the field the caller can shorten, so the refusal is a 422
// against `links` rather than an unattributed rejection.
func (e *TooManyLinksError) FieldFault() (field, code, message string) {
	return fieldLinks, "too_many_links", e.Error()
}

// insertActivityLinks writes the polymorphic link rows. The last_activity_at
// clocks on deal, contact and company move with them, but not from here:
// migration 1787032690 keeps them on the activity_link row itself (a trigger
// recomputing from the timeline), because this is one of several writers of
// that row — capture, ensure, relink and message identity insert links too —
// and the reach set also moves with no link written at all (an employment
// starts or ends, a deal moves account, an activity is archived or re-dated).
//
// The FK alone is not enough: it is
// checked as the table owner, bypassing RLS, so it would accept a
// guessed cross-tenant or out-of-scope UUID as a link target — every
// target passes the row-scope link check first.
//
// A repeated (type, id) is written ONCE. uq_activity_link already says a link
// is one row per record, so a caller that named the same company twice was
// answered with a unique violation — a 500 at the end of a send or a booking,
// and for an agent's approved retry a 500 that consumed the human's one-shot
// approval on a message that then never left. Deduplicating here rather than in
// each caller is what makes that true of every transport at once.
//
// The count is BOUNDED here for the same reason: the list is the caller's to
// choose, every entry costs its own row-scope probe and its own insert, and
// this is the one statement every writer passes through — the timeline's link
// vocabulary describes what a message or meeting is ABOUT, and a record set
// larger than this is about nothing.
//
// A PROJECT link additionally requires activity.UPDATE, which the create grant
// alone does not confer. Filing under a project classifies the correspondence
// as commercial (D5), and that classification is write-once in the database and
// is not lifted by unfiling — so it is a heavier act than creating a row, and
// the capture ladder already refuses it on exactly these terms
// (capture/sinkproject.go: "a principal that may create captured mail but not
// change it attributes nothing"). Two doors onto one act must not disagree
// about who may perform it; before this the create door was the cheaper way in.
func insertActivityLinks(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, kind string, links []ActivityLinkInput) error {
	if len(links) > maxActivityLinks {
		return &TooManyLinksError{Count: len(links)}
	}
	seen := make(map[ActivityLinkInput]struct{}, len(links))
	for _, link := range links {
		if _, duplicate := seen[link]; duplicate {
			continue
		}
		seen[link] = struct{}{}
		column := linkColumn(link.EntityType)
		if column == "" {
			return &InvalidLinkTypeError{EntityType: link.EntityType}
		}
		// The project grant is checked BEFORE the insert it gates, not after.
		// A refusal returned later is undone only because every caller runs
		// this inside a transaction it aborts — correct today, and the wrong
		// thing to hang an authorization check on. The sibling writer asks
		// first (capture/sinkproject.go), and two doors onto one act must not
		// disagree about WHEN they ask either.
		if link.EntityType == linkEntityProject {
			if err := refuseAnUnattendedFiling(ctx); err != nil {
				return err
			}
			if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
				return err
			}
		}
		if err := auth.EnsureLinkTarget(ctx, tx, link.EntityType, link.EntityID); err != nil {
			return err
		}
		// AFTER the target probe, not before it. A caller naming a company they
		// cannot see must get the answer their row scope gives — the same
		// not-found every other unreachable target gets — rather than a
		// modelling refusal that tells them the id was real enough to judge.
		if err := refuseACompanyMeeting(kind, link); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			sprintf(`INSERT INTO activity_link (activity_id, entity_type, %s) VALUES ($1, $2, $3)`, column),
			activityID, link.EntityType, link.EntityID); err != nil {
			return err
		}
		if link.EntityType == linkEntityProject {
			if err := StampCorrespondenceForProject(ctx, tx, activityID, link.EntityID); err != nil {
				return err
			}
		}
	}
	return nil
}

// refuseACompanyMeeting answers the company link on a meeting or a call the way
// a caller can act on.
//
// The estate refuses it either way — a trigger, because activity_link is
// written by the MCP tool, a REST caller and the web app alike (migration
// 1788000100). What that refusal alone gives a caller is a check violation with
// no field on it: the tool surface renders it as "a value in this request is
// outside what its field accepts", which names neither the link nor what to do
// instead, so a model retries the same call or abandons the write. Named here,
// the answer says which array to change and what the company is reached
// through.
//
// The KIND is passed in rather than read back: the creating caller has it in
// hand, and a read of `activity` here would be a reader of the restricted table
// that excludes no held row. The relink path takes the trigger's answer
// instead — it holds no kind and would need that read to name one.
func refuseACompanyMeeting(kind string, link ActivityLinkInput) error {
	if !personalActivityKinds[kind] || link.EntityType != linkEntityCompany {
		return nil
	}
	return &CompanyMeetingError{Kind: kind}
}

// personalActivityKinds are the kinds that are inherently WITH A HUMAN, and so
// cannot be filed against a company.
//
// Email is deliberately absent: a mail can legitimately be addressed to an
// account alias nobody owns personally. `note` and `task` are absent for a
// different reason — they are ABOUT a record rather than with a contact, and a
// note about a company is exactly what a company timeline is for.
var personalActivityKinds = map[string]bool{"meeting": true, "call": true}

// CompanyMeetingError refuses a meeting or a call filed against a company.
type CompanyMeetingError struct{ Kind string }

func (e *CompanyMeetingError) Error() string {
	return fmt.Sprintf("a %s is with a contact, not with a company: link the contact who was there, "+
		"and the company is reached through their employer", e.Kind)
}

// FieldFault names the array the caller can correct, so the refusal is a 422
// against `links` rather than an unattributed check violation.
func (e *CompanyMeetingError) FieldFault() (field, code, message string) {
	return fieldLinks, "company_meeting", e.Error()
}

// InvalidLinkTypeError maps to 422.
type InvalidLinkTypeError struct{ EntityType string }

func (e *InvalidLinkTypeError) Error() string {
	return "activity link entity_type " + e.EntityType + " is not " + linkVocabulary()
}

// FieldFault refuses a link to an entity type the timeline does not carry.
func (e *InvalidLinkTypeError) FieldFault() (field, code, message string) {
	return fieldLinks, "invalid_entity_type", e.Error()
}

// refuseAnUnattendedFiling keeps the retention mark off the agent's create
// path, at the branch that writes it: the stamp below this one is what files
// an activity as commercial correspondence, and relink — the verb that moves
// an EXISTING activity — reaches its own writer rather than this one.
//
// Filing under a project classifies an activity as commercial correspondence:
// write-once in the database, monotonic, and removable only by a named contact
// giving a written reason. relink_activity was raised to confirm-first for a
// project destination for exactly that reason. The create path reaches the same
// write and five tools ride it — log_activity, create_task, book_meeting,
// draft_email, send_account_email — every one of them auto-execute. A passport
// holding activity:update could call any of them in a loop and mint one mark
// per call with nobody watching (#2266).
//
// Refused HERE rather than in each of the five, because five refusals are five
// chances to add a sixth tool and forget: #2266 exists because the tier fix
// covered relink and missed the create door beside it. Refusing at the write
// means a tool added tomorrow inherits the answer.
//
// Not by tier, which is the fix that cannot be made: a scheduled agent may only
// call auto-execute tools, because a run that suspends on a staged approval is
// still reported as `running` by the AI-activity projection, and
// overnight_at_risk_sweep logs a note per at-risk deal. Raising these would tell
// an operator the AI is working while it waits for them.
//
// A human at a form, a REST caller, and the capture sink are untouched — none
// of them is an agent principal, and having a contact in the loop is the whole
// distinction being drawn.
func refuseAnUnattendedFiling(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	return &UnattendedProjectFilingError{}
}

// UnattendedProjectFilingError refuses an agent filing a NEW activity under a
// project on the create path.
type UnattendedProjectFilingError struct{}

func (e *UnattendedProjectFilingError) Error() string {
	return "filing an activity under a project marks it as commercial correspondence, which is " +
		"write-once and removable only by a named contact giving a written reason — not a mark this " +
		"call may write unattended"
}

// FieldFault names the array to change and the verb that files under a project
// with a contact deciding it, because a refusal a caller cannot act on is one
// they retry unchanged.
func (e *UnattendedProjectFilingError) FieldFault() (field, code, message string) {
	return fieldLinks, "project_filing_needs_approval", e.Error() +
		". Create it with its other links, then call relink_activity for the project: a contact " +
		"approves that one before it takes effect"
}
