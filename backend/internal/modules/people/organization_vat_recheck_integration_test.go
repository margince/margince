// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Asking the register again.
//
// The automatic lanes consult only about a number they have not seen, so a
// stored verdict stood forever: a rep who knew a registration had changed at
// the registry had no way to find out. This is that way, and what it must not
// become is a button that spends the installation's shared rate every time
// somebody leans on it.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// vatCheckRecorder is the enqueue seam, capturing what the store asked for.
// Whether the job was marked as REQUESTED is the whole point: the worker reads
// that flag to decide whether an already-answered number is asked about again.
type vatCheckRecorder struct {
	calls []bool
}

func (r *vatCheckRecorder) enqueue() VatCheckEnqueue {
	return func(_ context.Context, _ pgx.Tx, _ ids.OrganizationID, requested bool) error {
		r.calls = append(r.calls, requested)
		return nil
	}
}

// statedNumber is the one VAT ID this suite consults about. A real, publicly
// valid German number, so a reader checking the fixture against the register
// finds what the tests assume.
const statedNumber = "DE811907980"

// orgStatingVat seeds a company whose VAT number a person stated — the real
// writer, so the profile-field row is the one production makes. Both id shapes
// come back because the store takes one and the handler takes the other.
func orgStatingVat(
	ctx context.Context, t *testing.T, e *dedupeEnv,
) (ids.OrganizationID, crmcontracts.Id) {
	number := statedNumber
	t.Helper()
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Belegpflicht GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	if _, err := e.store.UpdateOrganizationProfileField(ctx, orgID, "register_vat",
		ProfileFieldWriteInput{Value: &number}); err != nil {
		t.Fatalf("state the VAT number: %v", err)
	}
	return orgID, org.Id
}

// The case the product could not do: ask again about a number that has not
// changed. The automatic lanes decline exactly this, which is why the request
// carries a flag rather than relying on the number looking new.
func TestAPersonCanAskTheRegisterAgainAboutAnUnchangedNumber(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var recorder vatCheckRecorder
	e.store.WithVatCheckEnqueue(recorder.enqueue())
	orgID, _ := orgStatingVat(ctx, t, e)

	// The number is already answered, so the automatic rule would decline.
	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: orgID, Number: "DE811907980", Status: VatCheckValid,
		CheckedAt: consultedAt,
	}); err != nil {
		t.Fatalf("record the standing answer: %v", err)
	}

	if err := e.store.RequestVatCheck(ctx, orgID); err != nil {
		t.Fatalf("request a fresh consultation: %v", err)
	}

	// One queued by the write that stated the number, one by the request. The
	// second must say a person asked for it, or the worker declines it exactly
	// as it declines the automatic one.
	if len(recorder.calls) != 2 {
		t.Fatalf("queued %d consultations, want 2 (the write, then the request)", len(recorder.calls))
	}
	if recorder.calls[0] {
		t.Error("the write's own consultation was marked as requested — it was not asked for by a person")
	}
	if !recorder.calls[1] {
		t.Error("the person's request was queued unmarked, so the worker will decline it as already answered")
	}
}

// The floor. The register is a shared public service consulted on one worker,
// and a button with nothing under it is how an installation gets blocked for
// everybody.
func TestAskingAgainWithinTheCooldownIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var recorder vatCheckRecorder
	e.store.WithVatCheckEnqueue(recorder.enqueue())
	orgID, _ := orgStatingVat(ctx, t, e)

	// Consulted now, by the database's own clock — the one the cooldown is
	// measured against.
	var now time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT now()`).Scan(&now)
	}); err != nil {
		t.Fatalf("read the transaction clock: %v", err)
	}
	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: orgID, Number: "DE811907980", Status: VatCheckValid, CheckedAt: now,
	}); err != nil {
		t.Fatalf("record the fresh answer: %v", err)
	}

	queuedBefore := len(recorder.calls)
	err := e.store.RequestVatCheck(ctx, orgID)
	if !errors.Is(err, apperrors.ErrBudgetExceeded) {
		t.Fatalf("got %v, want the rate refusal — the answer on the record still stands", err)
	}
	if len(recorder.calls) != queuedBefore {
		t.Error("the refused request queued a consultation anyway")
	}
}

// A company nobody has stated a number for has nothing to consult, and that is
// a different fact from asking too often: one needs a number typed, the other
// needs a wait. Told apart, or the reader is sent to do the wrong thing.
func TestAskingAboutACompanyWithNoNumberIsNotFound(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var recorder vatCheckRecorder
	e.store.WithVatCheckEnqueue(recorder.enqueue())

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Ohne Nummer GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	if err := e.store.RequestVatCheck(ctx, orgID); !errors.Is(err, ErrVatCheckNotRecorded) {
		t.Fatalf("got %v, want not-recorded — there is no number to consult", err)
	}
	if len(recorder.calls) != 0 {
		t.Error("a company with no number queued a consultation")
	}
}

// A deployment that consults no register must refuse rather than accept: a
// queued job on a lane nothing reads answers the reader with a promise the
// installation cannot keep, and the button would appear to work forever.
func TestAskingOnADeploymentThatConsultsNoRegisterIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var recorder vatCheckRecorder
	e.store.WithVatCheckEnqueue(recorder.enqueue())
	orgID, _ := orgStatingVat(ctx, t, e)

	// The deployment loses its register between the write and the request.
	e.store.WithVatCheckEnqueue(nil)

	if err := e.store.RequestVatCheck(ctx, orgID); !errors.Is(err, ErrVatCheckNotRecorded) {
		t.Fatalf("got %v, want not-recorded — this installation consults nothing", err)
	}
}

// The request writes a receipt onto the record and spends a shared rate, so it
// takes the authority to CHANGE the record rather than merely read it. A seat
// that may read the company and not write it is refused.
func TestAskingTakesTheAuthorityToChangeTheRecord(t *testing.T) {
	e := setupDedupe(t)
	var recorder vatCheckRecorder
	e.store.WithVatCheckEnqueue(recorder.enqueue())
	orgID, _ := orgStatingVat(e.as(), t, e)

	if err := e.store.RequestVatCheck(e.asOrgReader(), orgID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("got %v, want permission denied", err)
	}
	if len(recorder.calls) != 1 {
		t.Error("the refused request queued a consultation")
	}
}

// asOrgReader may READ every company and write none — the seat that separates
// consulting the register from looking at what it said.
func (e *dedupeEnv) asOrgReader() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"organization": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The transport, so the button is reachable rather than merely implemented.
// 202 because the register can be slow or decline, and neither should hold the
// request open — the verdict is read back from the GET.
func TestRequestOrganizationVatCheckHandler(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var recorder vatCheckRecorder
	e.store.WithVatCheckEnqueue(recorder.enqueue())
	orgID, wireID := orgStatingVat(ctx, t, e)
	h := Handlers{store: e.store}
	req := httptest.NewRequest(http.MethodPost, "/organizations/x/vat-check", nil).WithContext(ctx)

	rec := httptest.NewRecorder()
	h.RequestOrganizationVatCheck(rec, req, wireID)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body: %s)", rec.Code, rec.Body.String())
	}

	// Asked again immediately: the reader is told to wait, not that something
	// is broken. 429 and 404 send a person to do different things.
	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: orgID, Number: "DE811907980", Status: VatCheckValid, CheckedAt: nowInTx(ctx, t, e),
	}); err != nil {
		t.Fatalf("record the fresh answer: %v", err)
	}
	rec = httptest.NewRecorder()
	h.RequestOrganizationVatCheck(rec, req, wireID)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429 (body: %s)", rec.Code, rec.Body.String())
	}
}

func nowInTx(ctx context.Context, t *testing.T, e *dedupeEnv) time.Time {
	t.Helper()
	var now time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT now()`).Scan(&now)
	}); err != nil {
		t.Fatalf("read the transaction clock: %v", err)
	}
	return now
}
