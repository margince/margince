// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The account-driven reach set answers what the timeline predicate answers,
// arm by arm, including the rows each arm's date and currency bounds refuse.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheReachedSetAgreesWithThePredicateOnEveryArm(t *testing.T) {
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
	beforeTheyJoined, leaverCopied, otherAccount := mail("before"), mail("leaver"), mail("other")
	link(beforeTheyJoined, "contact", joinedLater)
	copied(leaverCopied, leaver)
	link(otherAccount, "company", elsewhere)

	want := map[ids.UUID]bool{filed: true, onDeal: true, aboutEmployee: true, employeeCopied: true}
	for _, company := range []ids.UUID{account, elsewhere} {
		byPredicate := e.activityIDs(t, `SELECT a.id FROM activity a WHERE `+CompanyLinkedActivityExists(1), company)
		bySet := e.activityIDs(t, `SELECT reach.activity_id FROM (`+CompanyReachedActivities(1)+`) reach`,
			[]ids.UUID{company})
		if len(bySet) != len(byPredicate) {
			t.Errorf("company %s: the set reached %d activities and the predicate %d", company, len(bySet), len(byPredicate))
		}
		for id := range byPredicate {
			if !bySet[id] {
				t.Errorf("company %s: the predicate reaches %s and the set does not", company, id)
			}
		}
	}
	got := e.activityIDs(t, `SELECT reach.activity_id FROM (`+CompanyReachedActivities(1)+`) reach
		WHERE reach.company_id = $2`, []ids.UUID{account, elsewhere}, account)
	if len(got) != len(want) {
		t.Errorf("the account reached %d activities, want the 4 its arms admit: %v", len(got), got)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("the account does not reach %s, which one of its arms admits", id)
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
