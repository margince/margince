// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the worker does with a consultation a PERSON asked for.
//
// The automatic lanes ask only about a number they have not seen, which is what
// stops an enqueue-per-keystroke spending the installation's shared rate. That
// same rule is what made a stored verdict permanent: a rep who knew a
// registration had changed at the registry could not get it re-asked. The
// requested flag is the exception, and this is the pair of cases that keeps it
// an exception rather than a hole — a person's request asks again, and a write's
// does not.

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/vatcheck"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// countingRegister answers every consultation the same way and counts them.
// The count IS the assertion: whether the register was asked at all is exactly
// what the requested flag decides.
type countingRegister struct {
	asked   int
	numbers []string
}

func (c *countingRegister) Check(_ context.Context, number string) (vatcheck.Result, error) {
	c.asked++
	c.numbers = append(c.numbers, number)
	return vatcheck.Result{Status: vatcheck.StatusValid, ConsultationNumber: "WAPIAAAA"}, nil
}

// vatRecheckEnv is one workspace holding a company whose VAT number has already
// been consulted — the state in which the automatic rule declines.
type vatRecheckEnv struct {
	*integration.Env
	worker   *vatCheckWorker
	register *countingRegister
	orgID    ids.OrganizationID
}

func setupVatRecheck(t *testing.T) *vatRecheckEnv {
	t.Helper()
	e := integration.Setup(t)
	register := &countingRegister{}
	v := &vatRecheckEnv{
		Env:      e,
		register: register,
		worker: newVatCheckWorker(e.Pool, register, func() time.Time {
			return time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
		}),
	}

	ctx := e.Admin()
	store := e.People
	org, err := store.CreateOrganization(ctx, people.CreateOrganizationInput{
		DisplayName: "Belegpflicht GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	v.orgID = ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	// Seeded through the real writers: the number as a person states it, and the
	// answer as the worker records it. A hand-inserted pair proves nothing about
	// the rows production makes, and this test turns on those rows agreeing.
	number := "DE811907980"
	if _, err := store.UpdateOrganizationProfileField(ctx, v.orgID, "register_vat",
		people.ProfileFieldWriteInput{Value: &number}); err != nil {
		t.Fatalf("state the VAT number: %v", err)
	}
	if err := store.RecordVatCheck(ctx, people.VatCheck{
		OrganizationID: v.orgID, Number: number, Status: people.VatCheckValid,
		CheckedAt: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("record the standing answer: %v", err)
	}
	return v
}

func (v *vatRecheckEnv) work(t *testing.T, requested bool) {
	t.Helper()
	err := v.worker.Work(context.Background(), &river.Job[CheckOrganizationVatArgs]{
		Args: CheckOrganizationVatArgs{
			Workspace:      v.WS,
			OrganizationID: v.orgID.UUID,
			Requested:      requested,
		},
	})
	if err != nil {
		t.Fatalf("working the consultation (requested=%v): %v", requested, err)
	}
}

// The rule that made the verdict permanent, still in force for the lanes that
// nobody asked. A write queues a consultation about a number already answered;
// the worker leaves the register alone.
func TestAnAutomaticConsultationDoesNotReAskAnAnsweredNumber(t *testing.T) {
	v := setupVatRecheck(t)

	v.work(t, false)

	if v.register.asked != 0 {
		t.Errorf("the register was consulted %d time(s) about a number it had already answered", v.register.asked)
	}
}

// The exception, and the reason this change exists: a person pressing the button
// has said the stored answer is not good enough.
func TestAPersonsRequestReAsksAnAnsweredNumber(t *testing.T) {
	v := setupVatRecheck(t)

	v.work(t, true)

	if v.register.asked != 1 {
		t.Fatalf("the register was consulted %d time(s), want exactly 1", v.register.asked)
	}
	if v.register.numbers[0] != "DE811907980" {
		t.Errorf("consulted %q, want the number the company states", v.register.numbers[0])
	}
}

// The exception has one floor the flag does not lift: a company that states no
// number has nothing to consult, however loudly it is asked about. Without this
// the flag would send the register an empty string.
func TestAPersonsRequestStillAsksNothingWhenNoNumberIsStated(t *testing.T) {
	v := setupVatRecheck(t)
	ctx := v.Admin()
	if _, err := v.Pool.Exec(ctx,
		`DELETE FROM organization_profile_field WHERE organization_id = $1 AND field = 'register_vat'`,
		v.orgID.UUID); err != nil {
		t.Fatalf("clear the stated number: %v", err)
	}

	v.work(t, true)

	if v.register.asked != 0 {
		t.Errorf("the register was consulted %d time(s) about a company that states no number", v.register.asked)
	}
}
