// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A company's stored clock counts a contact while the account walk calls them
// current. That holds through a notice period and through a month-dated
// departure's month, and stops once an employment is asserted former.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// databaseToday is the database's current_date, the clock IsCurrentSQL reads.
func databaseToday(t *testing.T) time.Time {
	t.Helper()
	var today time.Time
	if err := OwnerConn(t).QueryRow(context.Background(), `SELECT current_date`).Scan(&today); err != nil {
		t.Fatalf("reading current_date: %v", err)
	}
	return today
}

// employmentTerms is the assertion and the end date one employment edge carries.
type employmentTerms struct {
	status    *string
	precision *string
	endedAt   *time.Time
}

// hireUnderTerms records a contact at a new company through the relationship store.
func hireUnderTerms(t *testing.T, e *Env, name string, job employmentTerms) (company, contact ids.UUID) {
	t.Helper()
	company = e.SeedCompany(t, name, &e.Rep1)
	contact = e.SeedContact(t, "Ann at "+name, &e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual",
		EmploymentStatus: job.status, EndedPrecision: job.precision, EndedAt: job.endedAt,
	}); err != nil {
		t.Fatalf("recording the employment at %s: %v", name, err)
	}
	return company, contact
}

// logNoteOn files a note against the contact through the activity store.
func logNoteOn(t *testing.T, e *Env, contact ids.UUID, at time.Time) {
	t.Helper()
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Call notes"), Source: "manual", OccurredAt: &at,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatalf("logging the note: %v", err)
	}
}

func TestTheCompanyClockCountsAContactUntilTheirEmploymentEnds(t *testing.T) {
	e := Setup(t)
	today := databaseToday(t)
	inAMonth := today.AddDate(0, 0, 30)
	firstOfThisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		job    employmentTerms
		counts bool
	}{
		{"serving notice", employmentTerms{status: new("current"), precision: new("day"), endedAt: &inAMonth}, true},
		{"leaving within this month", employmentTerms{precision: new("month"), endedAt: &firstOfThisMonth}, true},
		{"former with no end date", employmentTerms{status: new("former")}, false},
		{"leaving today", employmentTerms{precision: new("day"), endedAt: &today}, false},
	}
	mail := today.AddDate(0, 0, -3).Add(9 * time.Hour)
	for _, tc := range cases {
		company, contact := hireUnderTerms(t, e, tc.name, tc.job)
		logNoteOn(t, e, contact, mail)

		var clock *time.Time
		if err := OwnerConn(t).QueryRow(context.Background(),
			`SELECT last_activity_of_company($1)`, company).Scan(&clock); err != nil {
			t.Fatalf("%s: reading the company clock: %v", tc.name, err)
		}
		if counted := clock != nil && clock.Equal(mail); counted != tc.counts {
			t.Errorf("%s: last_activity_of_company = %v, want the contact's mail counted: %v", tc.name, clock, tc.counts)
		}
	}
}

func TestMailToAContactServingNoticeMovesTheStoredEmployerClock(t *testing.T) {
	e := Setup(t)
	today := databaseToday(t)
	inAMonth := today.AddDate(0, 0, 30)
	company, contact := hireUnderTerms(t, e, "Notice Co",
		employmentTerms{status: new("current"), precision: new("day"), endedAt: &inAMonth})
	if got := companyClock(t, e, company); got != nil {
		t.Fatalf("company clock = %v before any mail, want none", got)
	}

	mail := today.AddDate(0, 0, -1).Add(9 * time.Hour)
	logNoteOn(t, e, contact, mail)
	if got := companyClock(t, e, company); got == nil || !got.Equal(mail) {
		t.Fatalf("company clock = %v after mail to a contact serving notice, want %v", got, mail)
	}
}
