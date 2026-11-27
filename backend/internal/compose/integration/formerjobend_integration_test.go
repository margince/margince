// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A former job has ended, so its end day is today or earlier. A current job
// may end ahead, which is a notice period, and only marking it former is refused.
func TestAFormerJobCannotEndAfterToday(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	contactID := ids.From[ids.ContactKind](e.SeedContact(t, "Ann Former", &e.Rep1))
	companyID := ids.From[ids.CompanyKind](e.SeedCompany(t, "Former employer", nil))
	former := "former"
	ahead := time.Date(2099, time.December, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2015, time.June, 15, 0, 0, 0, 0, time.UTC)
	job := func(status *string, ended *time.Time) contacts.CreateRelationshipInput {
		return contacts.CreateRelationshipInput{
			Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
			Source: "manual", EmploymentStatus: status, EndedAt: ended,
		}
	}
	wantFutureRefusal := func(what string, err error) {
		t.Helper()
		refusal, ok := errors.AsType[*values.ParseError](err)
		if !ok || refusal.Field != "ended_at" || refusal.Code != "future" {
			t.Fatalf("%s: err = %v, want an ended_at/future refusal", what, err)
		}
	}

	_, err := e.Contacts.CreateRelationship(e.Admin(), job(&former, &ahead))
	wantFutureRefusal("a former job ending in 2099", err)

	notice, err := e.Contacts.CreateRelationship(e.Admin(), job(nil, &ahead))
	if err != nil {
		t.Fatalf("a current job with a notice period: %v", err)
	}
	_, err = e.Contacts.UpdateRelationship(e.Admin(), notice.ID, contacts.UpdateRelationshipInput{EmploymentStatus: &former})
	wantFutureRefusal("marking a job former that ends in 2099", err)

	ended, err := e.Contacts.UpdateRelationship(e.Admin(), notice.ID, contacts.UpdateRelationshipInput{EmploymentStatus: &former, EndedAt: &past})
	if err != nil {
		t.Fatalf("marking the job former with a past end: %v", err)
	}
	if ended.EmploymentStatus == nil || *ended.EmploymentStatus != former {
		t.Fatalf("status = %v, want former", ended.EmploymentStatus)
	}
}
