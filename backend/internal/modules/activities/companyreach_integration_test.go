// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The account reach admits each arm, both as the timeline's predicate and as
// the set. It refuses the rows each arm's date and current-employment bounds
// leave out.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheAccountReachAdmitsEachArmWithinItsBounds(t *testing.T) {
	e := setupPromises(t)
	account, elsewhere := ids.NewV7(), ids.NewV7()
	for _, company := range []ids.UUID{account, elsewhere} {
		e.exec(t, `INSERT INTO company (id, display_name, source, captured_by)
			VALUES ($1, $2, 'seed', 'system')`, company, "Reach "+company.String())
	}
	employee, joinedLater, leaver := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, contact := range []ids.UUID{employee, joinedLater, leaver} {
		e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, $2, 'seed', 'system')`, contact, "Reach "+contact.String())
	}
	e.exec(t, `INSERT INTO relationship (id, kind, contact_id, company_id, source, captured_by)
		VALUES ($1, 'employment', $2, $3, 'seed', 'system')`, ids.NewV7(), employee, account)
	e.exec(t, `INSERT INTO relationship (id, kind, contact_id, company_id, started_at, source, captured_by)
		VALUES ($1, 'employment', $2, $3, current_date, 'seed', 'system')`, ids.NewV7(), joinedLater, account)
	e.exec(t, `INSERT INTO relationship (id, kind, contact_id, company_id, employment_status, source, captured_by)
		VALUES ($1, 'employment', $2, $3, 'former', 'seed', 'system')`, ids.NewV7(), leaver, account)
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, company_id, source, captured_by)
		VALUES ($1, 'Reach deal', 'open', $2, $3, $4, $5, 'seed', 'system')`, deal, e.rep, pipeline, stage, account)

	mail := func(subject string) ids.UUID {
		id := ids.NewV7()
		e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'email', $2, now() - interval '30 days', 'seed', 'system')`, id, subject)
		return id
	}
	link := func(activity ids.UUID, column string, target ids.UUID) {
		e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, `+column+`_id)
			VALUES ($1, $2, '`+column+`', $3)`, ids.NewV7(), activity, target)
	}
	copied := func(activity, contact ids.UUID) {
		e.exec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'cc')`,
			activity, contact)
	}
	filed, onDeal, aboutEmployee, employeeCopied := mail("filed"), mail("deal"), mail("about"), mail("copied")
	link(filed, "company", account)
	link(onDeal, "deal", deal)
	link(aboutEmployee, "contact", employee)
	copied(employeeCopied, employee)
	beforeTheyJoined, copiedBeforeJoining := mail("before"), mail("copied before joining")
	leaverCopied, otherAccount := mail("leaver"), mail("other")
	link(beforeTheyJoined, "contact", joinedLater)
	copied(copiedBeforeJoining, joinedLater)
	copied(leaverCopied, leaver)
	link(otherAccount, "company", elsewhere)
	refused := []ids.UUID{beforeTheyJoined, copiedBeforeJoining, leaverCopied, otherAccount}

	want := map[ids.UUID]bool{filed: true, onDeal: true, aboutEmployee: true, employeeCopied: true}
	reads := map[string]map[ids.UUID]bool{
		"the predicate": e.activityIDs(t, `SELECT a.id FROM activity a WHERE `+CompanyLinkedActivityExists(1), account),
		"the set": e.activityIDs(t, `SELECT reach.activity_id FROM (`+CompanyReachedActivities(1)+`) reach
			WHERE reach.company_id = $2`, []ids.UUID{account, elsewhere}, account),
	}
	for read, got := range reads {
		if len(got) != len(want) {
			t.Errorf("%s reached %d activities on the account, want the 4 its arms admit: %v", read, len(got), got)
		}
		for id := range want {
			if !got[id] {
				t.Errorf("%s does not reach %s, which one of the account's arms admits", read, id)
			}
		}
		for _, id := range refused {
			if got[id] {
				t.Errorf("%s reaches %s, which every arm's bounds leave out", read, id)
			}
		}
	}
}

func (e *promiseEnv) activityIDs(t *testing.T, query string, args ...any) map[ids.UUID]bool {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("reading reached activities: %v", err)
	}
	defer rows.Close()
	out := map[ids.UUID]bool{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scanning a reached activity: %v", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading reached activities: %v", err)
	}
	return out
}
