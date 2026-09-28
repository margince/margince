// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which values the contact read marks as bought, driven through the real
// applier, and how that list agrees with what "Delete bought data" removes.

import (
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// boughtDomain is the employer the purchases name; it must already be a company
// for the applier to link anybody to it.
const boughtDomain = "bought-employer.example"

// boughtClaims is one purchase that fills a title, a profile link, a mobile
// number and the employer, on a contact holding none of them.
func boughtClaims(mobile string) []provider.Claim {
	return []provider.Claim{
		{Key: provider.ClaimCurrentEmployment, Value: []byte(
			`{"company_name":"Bought Employer","company_domain":"` + boughtDomain + `","job_title":"Head of Revenue"}`)},
		{Key: provider.ClaimLinkedInProfile, Value: []byte(`"https://www.linkedin.com/in/bought-subject"`)},
		{Key: provider.ClaimMobilePhones, Value: []byte(`[{"value":"` + mobile + `"}]`)},
	}
}

// boughtSubject makes an empty contact, the employer company, and applies one
// purchase to the contact through contacts.ApplyProviderClaims.
func boughtSubject(t *testing.T, e *Env) (ids.UUID, *contacts.Store) {
	t.Helper()
	store := contacts.NewStore(e.DB())
	if _, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Bought Employer", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: boughtDomain, IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}
	subject := ids.NewV7()
	execAsOwner(t, e, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Bought Subject', 'manual', 'human:x')`, subject)
	applyPurchase(t, e, subject, "surfe", boughtClaims("+4915112345678"))
	return subject, store
}

// applyPurchase runs one completed purchase through the applier.
func applyPurchase(t *testing.T, e *Env, subject ids.UUID, vendor string, claims []provider.Claim) {
	t.Helper()
	run := seedRun(t, e, subject, "completed", ids.NewV7().String(), false)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.Admin(), `UPDATE provider_run SET provider = $2 WHERE id = $1`, run, vendor); err != nil {
			return err
		}
		if err := contacts.WriteProviderClaims(e.Admin(), tx, run, subject.String(), vendor, claims,
			time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)); err != nil {
			return err
		}
		return contacts.ApplyProviderClaims(e.Admin(), tx, run, subject.String(), vendor, claims)
	}); err != nil {
		t.Fatal(err)
	}
}

// boughtTargets reads the contact as ctx sees it and returns its bought targets.
func boughtTargets(ctx context.Context, t *testing.T, store *contacts.Store, subject ids.UUID) marks {
	t.Helper()
	c, err := store.GetContact(ctx, ids.From[ids.ContactKind](subject), storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	if c.BoughtFields == nil {
		t.Fatal("the single-contact read carried no bought_fields at all")
	}
	out := marks{}
	for _, f := range *c.BoughtFields {
		out[f.Target] = true
	}
	return out
}

// marks is a set of bought targets.
type marks map[string]bool

// has reports whether any target of this kind (`phone`, `employment`, ...) is marked.
func (m marks) has(kind string) bool {
	for target := range m {
		if k, _, _ := strings.Cut(target, ":"); k == kind {
			return true
		}
	}
	return false
}

func phoneRowOf(t *testing.T, e *Env, subject ids.UUID) ids.UUID {
	t.Helper()
	var row ids.UUID
	queryAsOwner(t, e, `SELECT id FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL`, &row, subject)
	return row
}

// TestTheContactReadMarksWhatAPurchaseStillHolds: every value the purchase
// filled is listed, and each drops out once somebody replaces it.
func TestTheContactReadMarksWhatAPurchaseStillHolds(t *testing.T) {
	e := Setup(t)
	subject, store := boughtSubject(t, e)
	boughtPhone := phoneRowOf(t, e, subject)

	got := boughtTargets(e.Admin(), t, store, subject)
	for _, kind := range []string{"title", "linkedin", "phone", "employment"} {
		if !got.has(kind) {
			t.Errorf("%s was filled by the purchase and is not marked bought: %v", kind, got)
		}
	}
	if !got["phone:"+boughtPhone.String()] {
		t.Errorf("the phone mark does not name the row the purchase wrote: %v", got)
	}

	title := "Chief Revenue Officer"
	if _, err := store.UpdateContact(e.Admin(), ids.From[ids.ContactKind](subject),
		contacts.UpdateContactInput{Title: &title, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	got = boughtTargets(e.Admin(), t, store, subject)
	if got.has("title") {
		t.Error("a title a colleague typed over the bought one is still marked bought")
	}
	if !got.has("phone") {
		t.Error("editing the title unmarked the bought phone, which nobody touched")
	}

	if _, err := store.UpdateContact(e.Admin(), ids.From[ids.ContactKind](subject), contacts.UpdateContactInput{
		Phones: []contacts.ContactPhoneInput{{Phone: "+4930123456", PhoneType: "work", IsPrimary: true}},
		Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	if got = boughtTargets(e.Admin(), t, store, subject); got.has("phone") {
		t.Errorf("a number a colleague put in place of the bought one is marked bought: %v", got)
	}
}

// TestTheMarksAndTheRevertAgree: whatever the page marks, the revert removes,
// except an employment another provider also supports — bought, and kept.
func TestTheMarksAndTheRevertAgree(t *testing.T) {
	e := Setup(t)
	subject, store := boughtSubject(t, e)
	filledEdge := employmentMark(t, boughtTargets(e.Admin(), t, store, subject))
	if _, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Past Employer", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: "shared-history.example", IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}
	history := []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(
		`[{"company_name":"Past Employer","company_domain":"shared-history.example","job_title":"Engineer","ended_at":"2020-06"}]`)}}
	for _, vendor := range []string{"surfe", "second_provider"} {
		applyPurchase(t, e, subject, vendor, history)
		if _, err := store.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](subject),
			crmcontracts.EmploymentImportRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	before := boughtTargets(e.Admin(), t, store, subject)
	delete(before, "employment:"+filledEdge)
	sharedEdge := employmentMark(t, before)

	var reverted contacts.RevertedSubject
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) (err error) {
		reverted, err = contacts.RevertProviderFills(e.Admin(), tx, "surfe", subject)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	cleared := strings.Join(reverted.Fields, ",")
	for _, kind := range []string{"title", "linkedin", "phone", "employment"} {
		if !strings.Contains(cleared, kind) {
			t.Errorf("%s was marked bought and the revert left it: %v", kind, reverted.Fields)
		}
	}
	if !archivedEdge(t, e, filledEdge) || archivedEdge(t, e, sharedEdge) {
		t.Error("the revert must retire the employment only surfe bought and keep the one " +
			"second_provider's purchase also supports")
	}
	after := boughtTargets(e.Admin(), t, store, subject)
	if len(after) != 1 || !after["employment:"+sharedEdge] {
		t.Errorf("after the revert only the shared employment may stay marked, got %v", after)
	}
}

// employmentMark returns the relationship id of the one employment marked.
func employmentMark(t *testing.T, m marks) string {
	t.Helper()
	for target := range m {
		if id, found := strings.CutPrefix(target, "employment:"); found {
			return id
		}
	}
	t.Fatalf("no employment is marked bought: %v", m)
	return ""
}

func archivedEdge(t *testing.T, e *Env, edge string) bool {
	t.Helper()
	var archived bool
	queryAsOwner(t, e, `SELECT archived_at IS NOT NULL FROM relationship WHERE id = $1`, &archived, edge)
	return archived
}

// TestABoughtEmployerTheReaderCannotSeeIsNotListed: an edge discloses its
// company, so a seat without the company grant gets no entry for it.
func TestABoughtEmployerTheReaderCannotSeeIsNotListed(t *testing.T) {
	e := Setup(t)
	subject, store := boughtSubject(t, e)
	objects := maps.Clone(AdminPerms.Objects)
	delete(objects, "company")
	blind := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: AdminPerms.RoleKeys, Objects: objects, RowScope: principal.RowScopeAll,
	})
	got := boughtTargets(blind, t, store, subject)
	if got.has("employment") {
		t.Errorf("a reader without the company grant learned of the bought employer: %v", got)
	}
	if !got.has("title") {
		t.Error("the title needs no company grant and lost its mark")
	}
}

// TestDeletingBoughtDataUnmarksWhatItClearedAndKeepsWhatItCouldNot, then the
// erasure removes what is left.
func TestDeletingBoughtDataUnmarksWhatItClearedAndKeepsWhatItCouldNot(t *testing.T) {
	e := Setup(t)
	cleared, store := boughtSubject(t, e)
	unreachable := ids.NewV7()
	execAsOwner(t, e, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Kept Subject', $2, 'manual', 'human:x')`, unreachable, e.Rep2)
	applyPurchase(t, e, unreachable, "surfe", boughtClaims("+4915187654321"))
	execAsOwner(t, e, `UPDATE contact SET owner_id = $1 WHERE id = $2`, e.AdminUser, cleared)

	narrow := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: AdminPerms.RoleKeys, Objects: providerAdminObjects(), RowScope: principal.RowScopeOwn,
	})
	integrationStore := providerStoreFor(t, e)
	if err := integrationStore.DeleteProviderData(narrow, "surfe"); err != nil {
		t.Fatal(err)
	}
	if got := boughtTargets(e.Admin(), t, store, cleared); len(got) != 0 {
		t.Errorf("values the deletion cleared are still marked: %v", got)
	}
	if got := boughtTargets(e.Admin(), t, store, unreachable); !got.has("title") {
		t.Errorf("a contact the deletion could not write kept its bought title but lost the mark: %v", got)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), unreachable, "test"); err != nil {
		t.Fatal(err)
	}
	if got := boughtTargets(e.Admin(), t, store, unreachable); len(got) != 0 {
		t.Errorf("an erased contact still names bought values: %v", got)
	}
}
