// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"
)

// Who a piece of correspondence was WITH, and how an installation that got the
// answer wrong is repaired.

// isConversation says whether an activity is with somebody. A note or a task
// is internal: it is about the account, not with a person.
func isConversation(kind string) bool {
	return kind == "email" || kind == "call" || kind == "meeting"
}

// relinkActivitiesToPeople repoints the activities already on file at the
// person who actually signed the mail.
//
// The same reason relinkActivitiesToProjects exists, for the defect one field
// over. Posting an activity again does NOT re-link it — the create path is
// idempotent on source_system+source_id and a replay returns the existing row
// before it reaches insertActivityLinks — so #2767's per-activity counterpart
// reached a FRESH database only. Every installation seeded before it keeps
// each activity attached to the account's most senior employee, which is the
// heuristic that fix exists to replace.
//
// That is not a cosmetic difference. Asked who raised a complaint, an
// assistant reads the sign-off in the body and answers with that name while
// activity_link names somebody else — an answer that looks like a
// hallucination and is a data contradiction. On an existing database the
// contradiction survives the fix, silently, and looks identical to a database
// that was never fixed.
func relinkActivitiesToPeople(
	c *client, cfg demoConfig, refs pipelineRefs,
	seen map[string]seededActivity, counterparts map[int]string, mode runMode,
) error {
	if mode == modeDryRun {
		return nil
	}
	for i, act := range cfg.Activities {
		existing, ok := seededMatch(refs, act, seen, i)
		if !ok {
			continue
		}
		want, move := personRelinkFor(act, existing, counterparts[i])
		if !move {
			continue
		}
		// replace_existing_of_type, because this is a MOVE: the link an older
		// seeder wrote names the wrong human, and leaving it beside the right
		// one would say the mail was with both. personRelinkFor has already
		// refused every activity carrying more than one, so nothing here
		// displaces a link this tool did not write.
		body := jsonBody{
			"entity_type":              "person",
			"entity_id":                want,
			"replace_existing_of_type": true,
		}
		if err := relinkPinned(c, existing, body); err != nil {
			return fmt.Errorf("filing activity %d (%s on %s) against its counterpart: %w", i, act.Kind, act.Company, err)
		}
	}
	return nil
}

// personRelinkFor decides whether an activity already on file is attached to
// the wrong person, and to whom it should move.
//
// Separate from the loop that performs it for the reason projectRelinkFor is:
// the decision is the part worth testing, and every relink is a write on a
// surface that stamps a six-year retention class the database will not let
// anyone lift — so "do nothing" is the answer that has to be right most often.
//
// `want` is the counterpart the CREATE path resolved this run, handed in
// rather than derived again. Two derivations of "who was this with" is one
// more than the question has, and the second one disagreeing would relink
// activities nothing had changed.
func personRelinkFor(act demoActivity, existing seededActivity, want string) (personID string, move bool) {
	// A note or a task is with nobody, and an account with no staff at all
	// gives no counterpart to move to. Neither is a repair; both are the
	// create path's own answer, unchanged.
	if !isConversation(act.Kind) || want == "" {
		return "", false
	}
	switch len(existing.PersonIDs) {
	case 0:
		// Never linked to anybody — the shape an installation seeded before
		// activities carried a person link is in.
		return want, true
	case 1:
		if existing.PersonIDs[0] == want {
			// Converged. The pass must be silent.
			return "", false
		}
		return want, true
	default:
		// MORE than one, which this tool never writes: the extra links came
		// from somewhere else — a capture that resolved participants, or
		// somebody associating a colleague by hand — and a move would delete
		// them. Leaving a wrong link standing is recoverable; deleting a fact
		// this tool did not create is not.
		return "", false
	}
}

// relinkPinned performs one repair against the version the snapshot was read
// at, and treats a version conflict as a row to leave alone.
//
// The pin is the answer to the window this pass would otherwise carry. The
// links it decides from were read in one pass at the start; the repair happens
// later, and `replace_existing_of_type` DELETES what it finds. Somebody adding
// a participant in between — a capture resolving one, a colleague associating
// themselves — would lose it to a decision made before their link existed.
// personRelinkFor refuses an activity carrying more than one person link, but
// only as the snapshot saw it.
//
// If-Match closes that against every writer that goes through the API: a relink
// touches the activity row so its version moves, and a version this snapshot did
// not see answers 409 and writes nothing. What it does not close is a link
// written straight into activity_link without touching the activity — nothing
// in the product does that today, and a seeding tool an operator runs is not
// where that would first matter.
//
// A conflict is a SKIP rather than a failure. The row moved under a converging
// tool; the next run reads it fresh and decides again.
func relinkPinned(c *client, existing seededActivity, body jsonBody) error {
	err := c.postGuarded("/v1/activities/"+existing.ID+"/relink", existing.Version, body, nil)
	if isConflict(err) {
		return nil
	}
	return err
}

// activityIdentityMatches answers whether a stored row really is the row a
// dataset entry is about.
//
// Source ids here are the entry's POSITION ("act-0", "act-1") and the dataset
// gives activities no ref of their own, so inserting an entry in the middle of
// the array renames every row after it. Two checks, because each sees what the
// other cannot: the ORGANIZATION catches a shift across companies, and the
// SUBJECT catches one within a single company, which is invisible to the
// organization check.
//
// orgID is what the caller could resolve, and "" means it could not — the
// post-seed verification has no domain map — so that half is skipped rather
// than failed. A dataset entry with no subject of its own is judged on the
// organization alone, which is what a call or a meeting usually carries.
func activityIdentityMatches(act demoActivity, existing seededActivity, orgID string) bool {
	if orgID != "" && existing.OrganizationID != orgID {
		return false
	}
	return act.Subject == "" || existing.Subject == act.Subject
}

// seededMatch answers whether a dataset entry's row is on file AND is really
// the row that entry is about.
//
// Shared by both reconciliation passes, because the question is one question
// and the consequence of getting it wrong is the same either way: acting on a
// mismatched id files one entry's mail against another entry's people or
// projects and stamps it with retention nobody can lift, so a disagreement
// means leave it alone. activityIdentityMatches is what that disagreement is.
func seededMatch(refs pipelineRefs, act demoActivity, seen map[string]seededActivity, i int) (seededActivity, bool) {
	existing, ok := seen[fmt.Sprintf("act-%d", i)]
	if !ok || existing.ID == "" {
		// Not on file before this run, so the create linked it.
		return seededActivity{}, false
	}
	orgID, known := refs.orgsByDom[strings.ToLower(act.Company)]
	if !known || !activityIdentityMatches(act, existing, orgID) {
		return seededActivity{}, false
	}
	return existing, true
}
