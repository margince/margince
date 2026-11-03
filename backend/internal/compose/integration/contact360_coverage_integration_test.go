// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestContactEvidenceSurvivesAnUnavailableCaptureObservation(t *testing.T) {
	e := Setup(t)
	id := e.SeedContact(t, "Coverage Fixture", &e.Rep1)
	clock := func() time.Time { return roomFixedNow }
	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), clock, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()), clock,
	).WithMailCoverage(func(context.Context, *time.Time, time.Time) (crmcontracts.MailCaptureCoverage, error) {
		return crmcontracts.MailCaptureCoverage{}, errors.New("fixture capture boundary unavailable")
	})
	out, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](id))
	if err != nil || out.Contact.Id != crmcontracts.Id(id) {
		t.Fatalf("capture failure hid contact evidence: %+v, %v", out.Contact, err)
	}
	if out.MailCoverage == nil || out.MailCoverage.State != crmcontracts.MailCaptureCoverageStateUnknown || out.MailCoverage.EvaluatedAt != roomFixedNow {
		t.Fatalf("capture failure was not qualified: %+v", out.MailCoverage)
	}
}
