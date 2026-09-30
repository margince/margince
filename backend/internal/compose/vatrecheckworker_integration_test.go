// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the worker does with a consultation a CONTACT asked for.
//
// The automatic lanes ask only about a number they have not seen, which is what
// stops an enqueue-per-keystroke spending the installation's shared rate. That
// same rule is what made a stored verdict permanent: a rep who knew a
// registration had changed at the registry could not get it re-asked. The
// requested flag is the exception, and this is the pair of cases that keeps it
// an exception rather than a hole — a contact's request asks again, and a write's
// does not.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
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

// A value that is not VAT-ID shaped is settled before any request: the client
// refuses it rather than spending somebody else's service on it.
type refusingRegister struct{ asked int }

func (r *refusingRegister) Check(_ context.Context, _ string) (vatcheck.Result, error) {
	r.asked++
	return vatcheck.Result{}, vatcheck.ErrMalformedNumber
}

// vatRecheckEnv is one workspace holding a company whose VAT number has already
// been consulted — the state in which the automatic rule declines.
type vatRecheckEnv struct {
	*integration.Env
	worker    *vatCheckWorker
	register  *countingRegister
	companyID ids.CompanyID
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
	store := e.Contacts
	company, err := store.CreateCompany(ctx, contacts.CreateCompanyInput{
		DisplayName: "Belegpflicht GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	v.companyID = ids.From[ids.CompanyKind](ids.UUID(company.Id))

	// Seeded through the real writers: the number as a contact states it, and the
	// answer as the worker records it. A hand-inserted pair proves nothing about
	// the rows production makes, and this test turns on those rows agreeing.
	number := "DE811907980"
	if _, err := store.UpdateCompanyProfileField(ctx, v.companyID, "register_vat",
		contacts.ProfileFieldWriteInput{Value: &number}); err != nil {
		t.Fatalf("state the VAT number: %v", err)
	}
	if err := store.RecordVatCheck(ctx, contacts.VatCheck{
		CompanyID: v.companyID, Number: number, Status: contacts.VatCheckValid,
		CheckedAt: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("record the standing answer: %v", err)
	}
	return v
}

func (v *vatRecheckEnv) work(t *testing.T, requested bool) {
	t.Helper()
	err := v.worker.Work(context.Background(), &river.Job[CheckCompanyVatArgs]{
		Args: CheckCompanyVatArgs{
			Workspace: v.WS,
			CompanyID: v.companyID.UUID,
			Requested: requested,
		},
	})
	if err != nil {
		t.Fatalf("working the consultation (requested=%v): %v", requested, err)
	}
}

// A number that cannot be a VAT ID reads as INVALID, not as the register having
// declined.
//
// It was recorded as unanswered on the reasoning that no request was made, so
// the register said nothing — true, and the wrong fact to put in front of a
// reader. Somebody looking at their own typo was told a public service had
// failed them, and the thing they had to fix was on their own screen.
func TestANumberThatIsNotAVatIdIsInvalid(t *testing.T) {
	v := setupVatRecheck(t)
	register := &refusingRegister{}
	v.worker = newVatCheckWorker(v.Pool, register, func() time.Time {
		return time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	})
	ctx := v.Admin()
	// Stated by a human, through the real writer — the shape a rep produces
	// when they mistype into the field.
	malformed := "122323235sdf"
	if _, err := v.Contacts.UpdateCompanyProfileField(ctx, v.companyID, "register_vat",
		contacts.ProfileFieldWriteInput{Value: &malformed}); err != nil {
		t.Fatalf("state the malformed number: %v", err)
	}

	v.work(t, true)

	if register.asked != 1 {
		t.Fatalf("the client was consulted %d time(s), want 1 — it is what judges the shape", register.asked)
	}
	stored, err := v.Contacts.VatCheckFor(ctx, v.companyID)
	if err != nil {
		t.Fatalf("read the recorded answer: %v", err)
	}
	if stored.Status != contacts.VatCheckInvalid {
		t.Errorf("status = %q, want invalid — a reader fixes their own number either way", stored.Status)
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

// The exception, and the reason this change exists: a contact pressing the button
// has said the stored answer is not good enough.
func TestAContactsRequestReAsksAnAnsweredNumber(t *testing.T) {
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
func TestAContactsRequestStillAsksNothingWhenNoNumberIsStated(t *testing.T) {
	v := setupVatRecheck(t)
	ctx := v.Admin()
	if _, err := v.Pool.Exec(ctx,
		`DELETE FROM company_profile_field WHERE company_id = $1 AND field = 'register_vat'`,
		v.companyID.UUID); err != nil {
		t.Fatalf("clear the stated number: %v", err)
	}

	v.work(t, true)

	if v.register.asked != 0 {
		t.Errorf("the register was consulted %d time(s) about a company that states no number", v.register.asked)
	}
}

// movingRegister answers, and changes the stated number WHILE it answers — the
// correction landing in the window this job holds.
//
// It writes through the real profile-field writer, because that write is the
// one whose successor River deduplicates away: a hand-inserted value would
// prove the loop re-reads without proving there is anything for it to find.
type movingRegister struct {
	asked   int
	numbers []string
	moveTo  string
	move    func(number string) error
}

func (m *movingRegister) Check(_ context.Context, number string) (vatcheck.Result, error) {
	m.asked++
	m.numbers = append(m.numbers, number)
	if m.asked == 1 && m.move != nil {
		if err := m.move(m.moveTo); err != nil {
			return vatcheck.Result{}, err
		}
	}
	return vatcheck.Result{Status: vatcheck.StatusValid, ConsultationNumber: "WAPIAAAA"}, nil
}

// A number corrected while the job holds it is answered before the job ends.
//
// River's uniqueness spans `running` and must — a narrower set is refused
// outright — so the write's successor is skipped. Nothing else asks: this lane
// runs no sweep, deliberately. Without the second round the stored answer is
// about a number the company no longer states, and the next consultation waits
// on somebody editing the field again.
func TestANumberCorrectedWhileTheJobHoldsItIsStillAnswered(t *testing.T) {
	v := setupVatRecheck(t)
	const corrected = "DE136695976"
	register := &movingRegister{
		moveTo: corrected,
		move: func(number string) error {
			_, err := v.Contacts.UpdateCompanyProfileField(v.Admin(), v.companyID, "register_vat",
				contacts.ProfileFieldWriteInput{Value: &number})
			return err
		},
	}
	v.worker = newVatCheckWorker(v.Pool, register, func() time.Time {
		return time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	})

	v.work(t, true)

	if register.asked != 2 {
		t.Fatalf("the register was asked %d time(s) about %v — a value that moved under the job "+
			"is never asked about again, because the successor the write enqueued was "+
			"deduplicated against this very job", register.asked, register.numbers)
	}
	if register.numbers[1] != corrected {
		t.Errorf("the second consultation asked about %q, not the corrected %q",
			register.numbers[1], corrected)
	}
	// And what the row holds is the answer to what the company states now.
	check, err := v.Contacts.VatCheckFor(v.Admin(), v.companyID)
	if err != nil {
		t.Fatalf("reading the recorded check: %v", err)
	}
	if check.Number != corrected {
		t.Errorf("the stored check is about %q while the company states %q — the reader is shown "+
			"a verdict on a number nobody claims", check.Number, corrected)
	}
}

// And a number nobody touched costs ONE consultation, not two: the second round
// asks whether anything moved and stops when nothing did.
//
// The bound exists so a contact editing the field cannot turn one job into a
// run of consultations; this is the other half — the common case must not pay
// for the rare one, against a register running on goodwill.
func TestAnUnchangedNumberIsConsultedOnce(t *testing.T) {
	v := setupVatRecheck(t)

	v.work(t, true)

	if v.register.asked != 1 {
		t.Errorf("the register was asked %d times about a number nobody changed", v.register.asked)
	}
}

// throttledRegister tells us when to come back, which is the one refusal that
// is not a failure.
type throttledRegister struct {
	asked int
	after time.Duration
}

func (r *throttledRegister) Check(_ context.Context, _ string) (vatcheck.Result, error) {
	r.asked++
	return vatcheck.Result{}, &vatcheck.ProviderRefusedError{Status: 429, RetryAfter: r.after}
}

// A register that asks us to come back RESCHEDULES, and does not spend the
// job's second round on a service that just declined.
//
// The bound exists to stop one job becoming a run of consultations; a throttle
// is the case where even a second consultation is one too many, and coming back
// sooner than told is how an installation gets blocked outright.
func TestAThrottledRegisterReschedulesRatherThanAskingAgain(t *testing.T) {
	v := setupVatRecheck(t)
	register := &throttledRegister{after: 90 * time.Second}
	v.worker = newVatCheckWorker(v.Pool, register, func() time.Time {
		return time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	})

	err := v.worker.Work(context.Background(), &river.Job[CheckCompanyVatArgs]{
		Args: CheckCompanyVatArgs{Workspace: v.WS, CompanyID: v.companyID.UUID, Requested: true},
	})

	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatalf("a throttled register answered %v, want a snooze — the service named when to "+
			"come back, and anything else either drops the consultation or returns sooner "+
			"than it allowed", err)
	}
	if register.asked != 1 {
		t.Errorf("the register was asked %d times after telling us to wait", register.asked)
	}
}

// failingRegister cannot answer at all — a transport fault rather than a verdict.
type failingRegister struct{ asked int }

func (r *failingRegister) Check(_ context.Context, _ string) (vatcheck.Result, error) {
	r.asked++
	return vatcheck.Result{}, errors.New("dial tcp: connection refused")
}

// A consultation that did not complete is a fault, not a recorded answer, and it
// does not consume the second round either: River retries the job on its own
// ledger, which is where a transport failure belongs.
func TestAConsultationThatFailedRecordsNothingAndDoesNotRetryInLine(t *testing.T) {
	v := setupVatRecheck(t)
	register := &failingRegister{}
	v.worker = newVatCheckWorker(v.Pool, register, func() time.Time {
		return time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	})

	err := v.worker.Work(context.Background(), &river.Job[CheckCompanyVatArgs]{
		Args: CheckCompanyVatArgs{Workspace: v.WS, CompanyID: v.companyID.UUID, Requested: true},
	})
	if err == nil {
		t.Fatal("a register that could not be reached reported success, so River records the job " +
			"as done and nobody asks again")
	}
	if register.asked != 1 {
		t.Errorf("the register was asked %d times although the first attempt never completed",
			register.asked)
	}
	// And the standing answer is untouched: a failure is not a verdict.
	check, err := v.Contacts.VatCheckFor(v.Admin(), v.companyID)
	if err != nil {
		t.Fatalf("reading the standing check: %v", err)
	}
	if check.Status != contacts.VatCheckValid {
		t.Errorf("the standing answer became %q after a transport failure", check.Status)
	}
}
