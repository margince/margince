// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// "Needs attention" on a contact's page means that contact is waiting for us.
// These tests ask which pages one reply reminder lands on.

// mailContact seeds a contact reachable at `local`@customer.test; the rep's id keeps
// the address unique, because contact_email is deduplicated installation-wide.
func (e *loadEnv) mailContact(t *testing.T, local, visibility string) (ids.UUID, string) {
	t.Helper()
	contact, address := ids.NewV7(), local+"-"+e.rep.String()+"@customer.test"
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by)
		VALUES ($1, $2, $3, $4, 'seed', 'system')`, contact, local, e.other, visibility)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'seed', 'system')`, contact, address)
	return contact, address
}

func (e *loadEnv) company(t *testing.T) ids.UUID {
	t.Helper()
	company := ids.NewV7()
	e.exec(t, `INSERT INTO company (id, display_name, owner_id, source, captured_by)
		VALUES ($1, 'Customer GmbH', $2, 'seed', 'system')`, company, e.rep)
	return company
}

func contactLink(id ids.UUID) ActivityLinkInput {
	return ActivityLinkInput{EntityType: linkEntityContact, EntityID: id}
}

func companyLink(id ids.UUID) ActivityLinkInput {
	return ActivityLinkInput{EntityType: linkEntityCompany, EntityID: id}
}

// remindedOn files the reminder for `source` through the automatic pass, or
// through the rep taking it by hand, and returns every record it is linked to
// as "entity_type:id" — read whole, so a link the reader could not see still
// counts.
func remindedOn(t *testing.T, e *loadEnv, source ids.UUID, byHand bool) map[string]bool {
	t.Helper()
	store := storeKnowing(e)
	if byHand {
		request := LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source}
		if _, _, err := store.LogActivity(e.asSeat(e.rep), request); err != nil {
			t.Fatalf("taking the request: %v", err)
		}
	} else if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows, err := e.owner.Query(e.as(), `SELECT l.entity_type || ':' || (`+linkIDCoalesceQualified("l")+`)::text
		FROM activity_link l JOIN activity task ON task.id = l.activity_id
		WHERE task.source_system = 'email_request' AND task.source_activity_id = $1`, source)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	links := map[string]bool{}
	for rows.Next() {
		var link string
		if err := rows.Scan(&link); err != nil {
			t.Fatal(err)
		}
		links[link] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return links
}

func TestAReplyReminderSitsOnlyOnTheSendersPage(t *testing.T) {
	e := setupLoad(t)
	sender, from := e.mailContact(t, "sender", "workspace")
	firstCc, firstCcAddress := e.mailContact(t, "first-cc", "workspace")
	secondCc, secondCcAddress := e.mailContact(t, "second-cc", "workspace")
	// Linked by whoever logged the mail, but on none of its stated headers.
	unstated, _ := e.mailContact(t, "unstated", "workspace")
	company := e.company(t)
	source := seedEmailRequestFiledUnder(t, e, "Send the report", "commitment", OwedVerdictAsksUs, from,
		[]ActivityLinkInput{contactLink(sender), contactLink(firstCc), contactLink(secondCc), contactLink(unstated), companyLink(company)},
		mailParticipants{From: from, Cc: []string{firstCcAddress, secondCcAddress}})

	links := remindedOn(t, e, source, false)
	if len(links) != 1 || !links["contact:"+sender.String()] {
		t.Fatalf("reminder sits on %v, want only the sender %v (copied: %v, %v, unstated %v)", links, sender, firstCc, secondCc, unstated)
	}
	var role string
	if err := e.owner.QueryRow(e.as(), `SELECT role FROM activity_participant WHERE activity_id = $1 AND contact_id = $2`, source, unstated).Scan(&role); err != nil {
		t.Fatalf("a linked contact the headers do not name lost its participant row: %v", err)
	}
	if role != "cc" {
		t.Fatalf("a linked contact the headers do not name is recorded as %q, want cc", role)
	}
}

func TestAReplyReminderFromAnUnknownSenderSitsOnTheMailsRecords(t *testing.T) {
	e := setupLoad(t)
	copied, copiedAddress := e.mailContact(t, "copied", "workspace")
	company := e.company(t)
	stranger := "stranger-" + e.rep.String() + "@customer.test"
	source := seedEmailRequestFiledUnder(t, e, "Send the report", "commitment", OwedVerdictAsksUs, stranger,
		[]ActivityLinkInput{contactLink(copied), companyLink(company)},
		mailParticipants{From: stranger, Cc: []string{copiedAddress}})

	links := remindedOn(t, e, source, false)
	if len(links) != 1 || !links["company:"+company.String()] {
		t.Fatalf("reminder sits on %v, want only the company %v (copied contact %v)", links, company, copied)
	}
}

// A message whose sender was never recorded cannot tell the requester from the
// contacts copied, so the reminder keeps every link the message has.
func TestAReplyReminderOnAMailWithNoRecordedSenderKeepsEveryLink(t *testing.T) {
	e := setupLoad(t)
	first, firstAddress := e.mailContact(t, "first", "workspace")
	second, secondAddress := e.mailContact(t, "second", "workspace")
	company := e.company(t)
	source := seedEmailRequestFiledUnder(t, e, "Send the report", "commitment", OwedVerdictAsksUs, firstAddress,
		[]ActivityLinkInput{contactLink(first), contactLink(second), companyLink(company)},
		mailParticipants{Cc: []string{firstAddress, secondAddress}})

	links := remindedOn(t, e, source, false)
	want := []string{"contact:" + first.String(), "contact:" + second.String(), "company:" + company.String()}
	for _, link := range want {
		if !links[link] {
			t.Fatalf("reminder sits on %v, want every link of the mail %v", links, want)
		}
	}
	if len(links) != len(want) {
		t.Fatalf("reminder sits on %v, want exactly %v", links, want)
	}
}

// A sender kept private by a colleague is a page the assignee cannot open, so
// the reminder sits on the mail's other records instead — and the automatic
// pass and a rep taking the request by hand agree on that.
func TestBothDoorsFileAReminderFromAHiddenSenderOnTheSameRecords(t *testing.T) {
	for _, byHand := range []bool{false, true} {
		e := setupLoad(t)
		hidden, from := e.mailContact(t, "hidden", "owner")
		company := e.company(t)
		source := seedEmailRequestFiledUnder(t, e, "Send the report", "commitment", OwedVerdictAsksUs, from,
			[]ActivityLinkInput{contactLink(hidden), companyLink(company)},
			mailParticipants{From: from})

		links := remindedOn(t, e, source, byHand)
		if len(links) != 1 || !links["company:"+company.String()] {
			t.Fatalf("by hand %v: reminder sits on %v, want only the company %v (hidden sender %v)", byHand, links, company, hidden)
		}
	}
}

// A share to a team reaches its members only while the team is live: an
// archived team keeps its membership rows, and the seat a reminder is filed for
// is judged the way that seat itself is resolved.
func TestAReminderReachesASenderSharedWithTheAssigneesLiveTeamOnly(t *testing.T) {
	for _, archived := range []bool{false, true} {
		e := setupLoad(t)
		team := ids.NewV7()
		e.exec(t, `INSERT INTO team (id, name, archived_at) VALUES ($1, $2, CASE WHEN $3 THEN now() END)`, team, "team-"+team.String(), archived)
		e.exec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, team, e.rep)
		shared, from := e.mailContact(t, "shared", "owner")
		e.exec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ('contact', $1, 'team', $2, 'write', $3)`, shared, team, e.other)
		company := e.company(t)
		source := seedEmailRequestFiledUnder(t, e, "Send the report", "commitment", OwedVerdictAsksUs, from,
			[]ActivityLinkInput{contactLink(shared), companyLink(company)}, mailParticipants{From: from})

		want := "contact:" + shared.String()
		if archived {
			want = "company:" + company.String()
		}
		if links := remindedOn(t, e, source, false); len(links) != 1 || !links[want] {
			t.Fatalf("team archived %v: reminder sits on %v, want only %s", archived, links, want)
		}
	}
}
