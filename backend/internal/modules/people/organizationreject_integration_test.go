// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// "This is not a company", over a real Postgres.
//
// The verb replaced two HTTP calls, and every case here is about a property
// only one transaction has. The two-call version blocked the domain and then
// took a 403 on the archive, so a rep who held neither authority nor the second
// write left a refused domain behind a company that was still on every list —
// and the screen, which saw only the last error, told them nothing had
// happened.
//
// So each refusal asserts the SURVIVING state as well as the error: a refusal
// that wrote half of itself and reported failure is exactly the defect, and it
// looks identical to a clean refusal from the error alone.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asRejector is the seat that may reject: organization delete AND update, which
// are the archive and the standing domain decision.
func (e *dedupeEnv) asRejector() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"person":       {Create: true, Read: true, Update: true},
				"organization": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// seedCompanyOnDomain writes one company with one primary domain — the shape
// capture mints from mail, and the only shape this verb acts on.
func (e *dedupeEnv) seedCompanyOnDomain(ctx context.Context, t *testing.T, name, domain string) ids.OrganizationID {
	t.Helper()
	id := ids.New[ids.OrganizationKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization (id, display_name, owner_id, source, captured_by)
			VALUES ($1, $2, $3, 'manual', 'human:test')`, id, name, e.rep); err != nil {
			return err
		}
		if domain == "" {
			return nil
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO organization_domain (organization_id, domain, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'manual', 'human:test')`, id, domain)
		return err
	}); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return id
}

// admissionOf reads the standing decision a domain carries, or "" for none.
func (e *dedupeEnv) admissionOf(ctx context.Context, t *testing.T, domain string) (admission, source, reason string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT COALESCE(admission, ''), COALESCE(admission_source, ''), COALESCE(admission_reason, '')
			  FROM organization_domain_disposition WHERE domain = $1`, domain).
			Scan(&admission, &source, &reason)
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("reading the admission of %s: %v", domain, err)
	}
	return admission, source, reason
}

// archivedAt reports whether the company is retired.
func (e *dedupeEnv) archived(ctx context.Context, t *testing.T, id ids.OrganizationID) bool {
	t.Helper()
	var archived bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT archived_at IS NOT NULL FROM organization WHERE id = $1`, id).Scan(&archived)
	}); err != nil {
		t.Fatalf("reading whether %s is archived: %v", id, err)
	}
	return archived
}

// Both halves land, and the second half is the one that makes the first stick:
// the company does not come back on the next message from the same domain.
func TestRejectingACompanyArchivesItAndKeepsTheDomainFromMintingAnother(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asRejector()
	org := e.seedCompanyOnDomain(ctx, t, "Expensify Ltd", "expensify.test")

	out, err := e.store.RejectOrganization(ctx, org, "a tool we use, not a customer", nil)
	if err != nil {
		t.Fatalf("rejecting: %v", err)
	}

	if out.Organization.ArchivedAt == nil {
		t.Error("the answer's company is not archived — the caller is told the record survived")
	}
	if !e.archived(ctx, t, org) {
		t.Error("the company row is still live")
	}
	if out.Domain.Domain != "expensify.test" || out.Domain.Admission != DomainSuppressed {
		t.Errorf("the answer's decision is %+v, want expensify.test suppressed", out.Domain)
	}
	admission, source, reason := e.admissionOf(ctx, t, "expensify.test")
	if admission != DomainSuppressed {
		t.Errorf("the stored admission is %q, want %q", admission, DomainSuppressed)
	}
	// The source is what makes it STICKY. A machine source would let the next
	// bulk-sender verdict re-decide the domain a person just ruled on.
	if source != AdmissionSourceHuman {
		t.Errorf("the decision's source is %q, want %q — only a human decision outranks a later verdict",
			source, AdmissionSourceHuman)
	}
	if reason != "a tool we use, not a customer" {
		t.Errorf("the stored reason is %q, want the caller's", reason)
	}

	// The point of the second half, through the real capture path rather than
	// by re-reading the row this test just wrote: a message from the domain
	// creates the person and NO company.
	res, err := e.store.EnsureCounterparty(ctx,
		e.ensureInput(ctx, t, "billing@expensify.test", "Billing", "expensify.test"))
	if err != nil {
		t.Fatalf("a message from the refused domain: %v", err)
	}
	if res.OrganizationID != nil {
		t.Fatal("a message from the refused domain minted a company again — the refusal is what stops it coming back")
	}
}

// The authority both halves need, refused BEFORE either is written.
//
// This is the defect the two-call version could not fix: a rep holding
// organization update and not delete blocked the domain on the first call and
// took a 403 on the second. Here the refusal lands with nothing written, which
// is what one transaction buys.
func TestRejectingNeedsBothTheArchiveAndTheDomainAuthority(t *testing.T) {
	e := setupDedupe(t)
	seed := e.asRejector()
	org := e.seedCompanyOnDomain(seed, t, "Expensify Ltd", "expensify.test")

	// e.as() is the seeded rep: organization create/read/update, no delete.
	if _, err := e.store.RejectOrganization(e.as(), org, "a tool we use", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep holding update and not delete = %v, want ErrPermissionDenied", err)
	}
	if e.archived(seed, t, org) {
		t.Error("the company was archived by a refused rejection")
	}
	if admission, _, _ := e.admissionOf(seed, t, "expensify.test"); admission != "" {
		t.Errorf("the refused rejection left the domain %q — a suppression behind a company that is still there is the two-call defect", admission)
	}
}

// The domain is the company's CURRENT primary, read here.
//
// The closed attempt sent the domain the page was showing. With several
// domains, a primary that moved between the page load and the click suppressed
// the address nobody writes from any more, while the company came back
// tomorrow on the one that was never refused. There is no parameter to get
// wrong now, and this is the case that fails if one is ever added back.
func TestRejectingRefusesTheDomainTheCompanyUsesNow(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asRejector()
	org := e.seedCompanyOnDomain(ctx, t, "Kestner GmbH", "old.test")
	// The primary moves, exactly as it would between a page load and a click.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE organization_domain SET is_primary = false WHERE organization_id = $1`, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO organization_domain (organization_id, domain, is_primary, source, captured_by)
			VALUES ($1, 'new.test', true, 'manual', 'human:test')`, org)
		return err
	}); err != nil {
		t.Fatalf("moving the primary domain: %v", err)
	}

	out, err := e.store.RejectOrganization(ctx, org, "a vendor", nil)
	if err != nil {
		t.Fatalf("rejecting: %v", err)
	}

	if out.Domain.Domain != "new.test" {
		t.Errorf("refused %q, want new.test — the domain mail actually arrives on", out.Domain.Domain)
	}
	if admission, _, _ := e.admissionOf(ctx, t, "old.test"); admission != "" {
		t.Errorf("the domain the company stopped using carries %q — refusing it stops no mail and hides a decision nobody made", admission)
	}
}

// A company with no domain is refused, and told what does work.
//
// Somebody typed it in by hand, so it was never derived from mail and no
// refusal would stop anything. Archiving it silently would do half of what the
// button promises and say the other half happened.
func TestRejectingACompanyWithNoDomainIsRefusedAndLeavesItAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asRejector()
	org := e.seedCompanyOnDomain(ctx, t, "Typed By Hand Ltd", "")

	_, err := e.store.RejectOrganization(ctx, org, "a vendor", nil)
	var detailed *httperr.DetailedError
	if !errors.As(err, &detailed) {
		t.Fatalf("rejecting a domainless company = %v, want a validation refusal", err)
	}
	if len(detailed.Fields) != 1 || detailed.Fields[0].Code != "no_primary_domain" {
		t.Errorf("the refusal's fields are %+v, want one naming no_primary_domain", detailed.Fields)
	}
	// It must name the move that DOES work, or the reader is left with a
	// company they cannot get rid of and no idea why.
	if !strings.Contains(strings.ToLower(detailed.Detail), "archive") {
		t.Errorf("the refusal says %q — it must name the move that does work", detailed.Detail)
	}
	if e.archived(ctx, t, org) {
		t.Error("the company was archived by a refused rejection")
	}
}
