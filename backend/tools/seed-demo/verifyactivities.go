// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The two verification rules about WHO a conversation reached.
//
// Their own file because they are one concept and verify.go had outgrown the
// size cap: a mail filed against an account and nobody is a company timeline
// that fills while every contact's stays empty, and a mail filed against the
// WRONG person is the same page reading as complete while it contradicts the
// signature in its own body.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// checkActivitiesReachPeople catches correspondence filed against a company
// and nobody: the company timeline fills, every person's stays empty, and a
// rep opening a contact sees no history of talking to them.
func checkActivitiesReachPeople(c *client, _ demoConfig) ([]verifyFinding, error) {
	conversations, withPerson := 0, 0
	err := c.getAll("/v1/activities", nil, func(raw json.RawMessage) error {
		var rows []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Links []struct {
				EntityType string `json:"entity_type"`
			} `json:"links"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
		for _, act := range rows {
			// A note or a task is internal — about an account, not with
			// anybody. isConversation, not a third copy of the list: the
			// create path and the person repair both rule from it, and a
			// check disagreeing with what it checks is worse than no check.
			if !isConversation(act.Kind) {
				continue
			}
			conversations++
			for _, link := range act.Links {
				if link.EntityType == "person" {
					withPerson++
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if conversations == 0 || withPerson > 0 {
		return nil, nil
	}
	return []verifyFinding{{
		Rule:   "conversations name a person",
		Detail: fmt.Sprintf("%d mails/calls/meetings link to no person — every contact's timeline is empty", conversations),
	}}, nil
}

// checkConversationsNameTheRightPerson catches the installation the person
// reconciliation exists to repair: the mail is filed against SOMEBODY, so the
// rule above is satisfied, and it is the wrong human.
//
// That is the state an existing database was left in when activities started
// naming their counterpart per entry (#2767). The create path replays instead
// of relinking, so the link kept pointing at the account's most senior
// employee — and a database in that state is indistinguishable from a correct
// one by every count the seeder prints. Asked who raised a complaint, an
// assistant reads the sign-off in the body and answers with a name the CRM
// does not hold.
//
// Only entries the dataset NAMES a person for are judged. Everywhere else the
// senior contact is the honest heuristic and there is no right answer to check
// against. Two states are findings — filed against the wrong human, and filed
// against nobody at all — because the rule above this one is satisfied by any
// OTHER conversation having a person link, so a single unfiled mail is
// invisible to it.
//
// An activity carrying more than one person link is the one skip, for the
// reason personRelinkFor leaves it alone: those links did not come from here,
// and a rule reporting what the repair will never fix is one somebody turns
// off. A row whose identity does not match its dataset entry is skipped for
// the reason the repair skips it — positional source ids mean the entry may
// name a different row than it did last run, and judging it would report the
// wrong company's mail.
func checkConversationsNameTheRightPerson(c *client, cfg demoConfig) ([]verifyFinding, error) {
	onFile, err := loadActivitySourceIDs(c)
	if err != nil {
		return nil, err
	}
	var wrong []string
	for i, act := range cfg.Activities {
		if act.Person == "" || !isConversation(act.Kind) {
			continue
		}
		existing, ok := onFile[fmt.Sprintf("act-%d", i)]
		// Not on file at all is not a finding for THIS rule — the seed either
		// has not run or the row was removed, which the counts already say.
		// A row whose identity does not match is skipped for the reason the
		// repair skips it: positional source ids mean the entry may name a
		// different row than it did last time, and judging it would report the
		// wrong company's mail. The organization half cannot be checked here
		// (this runs with no domain map), so the subject carries it.
		if !ok || !activityIdentityMatches(act, existing, "") {
			continue
		}
		switch {
		case len(existing.PersonIDs) == 0:
			// The dataset says who signed it and the record says nobody. The
			// rule above this one is satisfied by any OTHER conversation
			// having a person, so without this the gap is invisible.
			wrong = append(wrong, fmt.Sprintf("%s signed by %s is filed against nobody", act.Company, act.Person))
		case len(existing.PersonIDs) > 1:
			// The only skip kept: the repair deliberately leaves these alone,
			// and a rule reporting what will never be fixed is one somebody
			// turns off.
			continue
		default:
			name, err := personName(c, existing.PersonIDs[0])
			if err != nil {
				return nil, err
			}
			if !strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(act.Person)) {
				wrong = append(wrong, fmt.Sprintf("%s signed by %s is filed against %s", act.Company, act.Person, name))
			}
		}
	}
	if len(wrong) == 0 {
		return nil, nil
	}
	return []verifyFinding{{
		Rule:   "conversations name the right person",
		Detail: fmt.Sprintf("%d filed against somebody other than who signed them (%s) — re-run the seeder, which repairs this", len(wrong), sample(wrong)),
	}}, nil
}
