// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An account reaches mail through a contact only for the time the contact
// worked there. Mail from someone's previous job must not land on the timeline
// of the employer they have now, nor be filed as a signal against it. It must
// not set that employer's last-activity clock either.
//
// Every walk case asks both shapes of the walk: the predicate the timeline reads
// (activities.CompanyLinkedActivityExists) and the set the signal producers
// file through (activities.CompanyReachSet). The edges are written by the real
// writers: capture's ensure for a first observation, the relationship store for
// a date a user supplied.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var (
	mailAtTheOldJob = time.Date(2024, time.May, 6, 9, 0, 0, 0, time.UTC)
	mailAtTheNewJob = time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)
)

// seedEmployerOnDomain is a company capture can attach a sender to.
func seedEmployerOnDomain(t *testing.T, e *Env, name, domain string) ids.UUID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: name, OwnerID: userIDPtr(&e.Rep1),
		Domains: []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.UUID(company.Id)
}

// seedMailFrom logs one captured message sent from address at a stated time,
// with the participant row capture writes for it.
func seedMailFrom(t *testing.T, address string, at time.Time) ids.UUID {
	t.Helper()
	owner := OwnerConn(t)
	id := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'hello', $2, 'gmail', 'connector:gmail')`, at)
	SeedIDRow(t, owner, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', $3)`, id, address)
	return id
}

// captureSender runs capture's ensure on one message, which links it to the
// sender and plants their employment edge.
func captureSender(t *testing.T, e *Env, activity ids.UUID, address, domain string) ids.UUID {
	t.Helper()
	res, err := e.Contacts.EnsureCounterparty(e.Admin(), contacts.EnsureCounterpartyInput{
		Email: address, Domain: domain, Replied: true,
		OwnerID: e.Rep1, ActivityID: ids.From[ids.ActivityKind](activity),
		Source: "gmail:" + activity.String(), CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("capturing %s: %v", address, err)
	}
	return res.ContactID.UUID
}

// seedOldJobMail is mail the contact sent from the employer they had before,
// filed against them and naming them on the message.
func seedOldJobMail(t *testing.T, contact ids.UUID) ids.UUID {
	t.Helper()
	owner := OwnerConn(t)
	id := seedMailFrom(t, "ann@oldco.test", mailAtTheOldJob)
	LinkActivity(t, owner, id, "contact", contact)
	LinkActivitySender(t, owner, id, contact)
	return id
}

// reaches answers whether activity reaches company, and fails the test when
// the timeline's predicate and the producers' set disagree about it.
func reaches(t *testing.T, e *Env, activity, company ids.UUID) bool {
	t.Helper()
	var predicate, set bool
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a
			 WHERE a.id = $2 AND `+activities.CompanyLinkedActivityExists(1)+`)`,
			company, activity).Scan(&predicate); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM (`+activities.CompanyReachSet()+`) reach
			 WHERE reach.activity_id = $1 AND reach.company_id = $2)`,
			activity, company).Scan(&set)
	}); err != nil {
		t.Fatalf("asking whether the activity reaches the account: %v", err)
	}
	if predicate != set {
		t.Fatalf("the timeline says %v and the producers' set says %v about one activity", predicate, set)
	}
	return predicate
}

func TestMailFromBeforeAnEdgeWasFirstObservedDoesNotReachTheEmployer(t *testing.T) {
	e := Setup(t)
	newco := seedEmployerOnDomain(t, e, "Newco", "newco.test")
	newJob := seedMailFrom(t, "ann@newco.test", mailAtTheNewJob)
	ann := captureSender(t, e, newJob, "ann@newco.test", "newco.test")
	oldJob := seedOldJobMail(t, ann)

	if !reaches(t, e, newJob, newco) {
		t.Fatal("the mail that placed the contact at the employer does not reach it")
	}
	if reaches(t, e, oldJob, newco) {
		t.Fatal("mail from the contact's previous job reaches an employer first seen two years later")
	}
}

// This case fails if NULL is ever read as "unknown". Every edge was undated
// before capture dated any, and an undated edge reaches all the contact's mail.
func TestMailReachesTheEmployerThroughAnUndatedEdge(t *testing.T) {
	e := Setup(t)
	newco := seedEmployerOnDomain(t, e, "Newco", "newco.test")
	ann := e.SeedContact(t, "Ann", &e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](ann), ids.From[ids.CompanyKind](newco)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual",
	}); err != nil {
		t.Fatalf("recording the employment: %v", err)
	}
	if !reaches(t, e, seedOldJobMail(t, ann), newco) {
		t.Fatal("an employment with no start date and no first observation no longer reaches the contact's mail")
	}
}

// A start date a user supplied outranks what capture observed, in either
// direction; this is the direction that widens.
func TestAStartDateAUserSuppliedWinsOverTheFirstObservation(t *testing.T) {
	e := Setup(t)
	newco := seedEmployerOnDomain(t, e, "Newco", "newco.test")
	newJob := seedMailFrom(t, "ann@newco.test", mailAtTheNewJob)
	ann := captureSender(t, e, newJob, "ann@newco.test", "newco.test")
	oldJob := seedOldJobMail(t, ann)

	var edge ids.UUID
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM relationship
			 WHERE kind = 'employment' AND contact_id = $1 AND company_id = $2
			   AND first_observed_at IS NOT NULL`, ann, newco).Scan(&edge)
	}); err != nil {
		t.Fatalf("finding the dated edge capture planted: %v", err)
	}
	started := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, err := e.Contacts.UpdateRelationship(ctx, edge, contacts.UpdateRelationshipInput{StartedAt: &started}); err != nil {
		t.Fatalf("supplying the start date: %v", err)
	}
	if !reaches(t, e, oldJob, newco) {
		t.Fatal("mail after the start date a user supplied does not reach the employer, " +
			"because the later first observation still bounds the edge")
	}
}

// companyClock is the company's stored last_activity_at, which the
// last_activity_of_company trigger function keeps.
func companyClock(t *testing.T, e *Env, company ids.UUID) *time.Time {
	t.Helper()
	read, err := e.Contacts.GetCompany(e.Admin(), companyIDOf(company), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the company: %v", err)
	}
	return read.LastActivityAt
}

// The stored clock counts the same mail the walk does. An undated edge counts
// all of it; a start date supplied later takes the older mail back out.
func TestTheCompanyClockCountsOnlyMailFromWhileTheContactWorkedThere(t *testing.T) {
	e := Setup(t)
	newco := seedEmployerOnDomain(t, e, "Newco", "newco.test")
	ann := e.SeedContact(t, "Ann", &e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](ann), ids.From[ids.CompanyKind](newco)
	edge, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("recording the employment: %v", err)
	}
	seedOldJobMail(t, ann)
	if got := companyClock(t, e, newco); got == nil || !got.Equal(mailAtTheOldJob) {
		t.Fatalf("company clock through an undated edge = %v, want %v", got, mailAtTheOldJob)
	}

	started := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, err := e.Contacts.UpdateRelationship(e.Admin(), edge.ID, contacts.UpdateRelationshipInput{StartedAt: &started}); err != nil {
		t.Fatalf("supplying the start date: %v", err)
	}
	if got := companyClock(t, e, newco); got != nil {
		t.Fatalf("company clock = %v after the start date moved past the only mail, want none", got)
	}
}

func TestTheCompanyClockIgnoresMailFromBeforeTheFirstObservation(t *testing.T) {
	e := Setup(t)
	newco := seedEmployerOnDomain(t, e, "Newco", "newco.test")
	newJob := seedMailFrom(t, "ann@newco.test", mailAtTheNewJob)
	ann := captureSender(t, e, newJob, "ann@newco.test", "newco.test")
	seedOldJobMail(t, ann)
	if got := companyClock(t, e, newco); got == nil || !got.Equal(mailAtTheNewJob) {
		t.Fatalf("company clock = %v, want %v from the mail that placed the contact there", got, mailAtTheNewJob)
	}

	// The edge keeps its first observation after that message is archived.
	// So only the older mail is left, and none of it counts.
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](newJob), nil); err != nil {
		t.Fatalf("archiving the new-job mail: %v", err)
	}
	if got := companyClock(t, e, newco); got != nil {
		t.Fatalf("company clock = %v from mail sent before the employer was first seen, want none", got)
	}
}

// A start date begins at UTC midnight whatever zone the session runs in. So a
// pooled connection with another TimeZone gives the same answer.
func TestAStartDateBoundsTheSameInEverySessionZone(t *testing.T) {
	e := Setup(t)
	newco := seedEmployerOnDomain(t, e, "Newco", "newco.test")
	ann := e.SeedContact(t, "Ann", &e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](ann), ids.From[ids.CompanyKind](newco)
	started := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual", StartedAt: &started,
	}); err != nil {
		t.Fatalf("recording the employment: %v", err)
	}
	firstMorning := started.Add(30 * time.Minute)
	mail := seedMailFrom(t, "ann@newco.test", firstMorning)
	owner := OwnerConn(t)
	LinkActivity(t, owner, mail, "contact", ann)
	LinkActivitySender(t, owner, mail, ann)

	for _, zone := range []string{"UTC", "America/Los_Angeles", "Asia/Tokyo"} {
		var walked bool
		var clock *time.Time
		ctx := e.Admin()
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('TimeZone', $1, true)`, zone); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM (`+activities.CompanyReachSet()+`) reach
				 WHERE reach.activity_id = $1 AND reach.company_id = $2)`, mail, newco).Scan(&walked); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT last_activity_of_company($1)`, newco).Scan(&clock)
		}); err != nil {
			t.Fatalf("asking in %s: %v", zone, err)
		}
		if !walked {
			t.Errorf("in session zone %s, mail sent on the start date does not reach the employer", zone)
		}
		if clock == nil || !clock.Equal(firstMorning) {
			t.Errorf("in session zone %s, the company clock reads %v, want %v", zone, clock, firstMorning)
		}
	}
}
