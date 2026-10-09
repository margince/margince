// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An account reaches mail through a contact only for the time the contact
// worked there. Mail from someone's previous job must not land on the timeline
// of the employer they have now, nor be filed as a signal against it.
//
// Every case asks both shapes of the walk: the predicate the timeline reads
// (activities.CompanyLinkedActivityExists) and the set the signal producers
// file through (activities.CompanyReachSet). The edges are written by the real
// writers: capture's ensure for a first observation, the relationship store for
// a date a person supplied.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
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

// A start date a person supplied outranks what capture observed, in either
// direction; this is the direction that widens.
func TestAStartDateAPersonSuppliedWinsOverTheFirstObservation(t *testing.T) {
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
		t.Fatal("mail after the start date a person supplied does not reach the employer, " +
			"because the later first observation still bounds the edge")
	}
}
