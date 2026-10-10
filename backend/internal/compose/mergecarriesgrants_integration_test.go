// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A merge carries the shares on the retired record onto the survivor. A
// grantee who held a share on both keeps one, with the higher access and the
// later expiry. Every change is audited on the record it changed.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// share writes one grant through the real writer.
func (c *carryEnv) share(t *testing.T, recordType string, record, grantee ids.UUID, access string, expires *time.Time) {
	t.Helper()
	if _, err := c.grants.CreateRecordGrant(c.admin, identity.CreateGrantInput{
		RecordType: recordType, RecordID: record, SubjectType: "user", SubjectID: grantee,
		Access: access, ExpiresAt: expires,
	}); err != nil {
		t.Fatalf("sharing the %s: %v", recordType, err)
	}
}

// grantOn reads the one grant a grantee holds on a record.
func (c *carryEnv) grantOn(t *testing.T, recordType string, record, grantee ids.UUID) (access string, expires *time.Time) {
	t.Helper()
	if err := c.e.Pool.QueryRow(context.Background(), `
		SELECT access, expires_at FROM record_grant
		 WHERE record_type = $1 AND record_id = $2 AND subject_type = 'user' AND subject_id = $3`,
		recordType, record, grantee).Scan(&access, &expires); err != nil {
		t.Fatalf("reading the grant on %s %s: %v", recordType, record, err)
	}
	return access, expires
}

func (c *carryEnv) companyNamed(t *testing.T, name string) ids.CompanyID {
	t.Helper()
	created, err := c.contacts.CreateCompany(c.admin, contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(created.Id))
}

func (c *carryEnv) grantsNaming(t *testing.T, recordType string, record ids.UUID) int {
	t.Helper()
	return c.count(t, `SELECT count(*) FROM record_grant WHERE record_type = $1 AND record_id = $2`, recordType, record)
}

func (c *carryEnv) mergeCompanies(t *testing.T, from, into ids.CompanyID) {
	t.Helper()
	if _, err := c.contacts.MergeCompany(c.admin, from, into, nil); err != nil {
		t.Fatalf("merging the companies: %v", err)
	}
}

// A grantee who could reach only the retired company reaches the survivor, and
// both records' histories say so.
func TestACompanyMergeMovesAShareOntoTheSurvivor(t *testing.T) {
	c := setupCarry(t)
	retired, survivor := c.companyNamed(t, "Moved Retired GmbH"), c.companyNamed(t, "Moved Survivor GmbH")
	c.share(t, "company", retired.UUID, c.e.Rep1, "write", nil)

	c.mergeCompanies(t, retired, survivor)

	if n := c.grantsNaming(t, "company", retired.UUID); n != 0 {
		t.Errorf("%d grants still name the retired company", n)
	}
	if access, expires := c.grantOn(t, "company", survivor.UUID, c.e.Rep1); access != "write" || expires != nil {
		t.Errorf("the moved grant is %s until %v, want write with no expiry", access, expires)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log
		WHERE action = 'record_unshare' AND entity_type = 'company' AND entity_id = $1
		  AND evidence->>'merged_into' = $2`, retired.UUID, survivor.UUID.String()); n != 1 {
		t.Errorf("%d unshare rows on the retired company name the survivor, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log
		WHERE action = 'record_share' AND entity_type = 'company' AND entity_id = $1
		  AND before IS NULL AND after->>'access' = 'write'
		  AND evidence->>'merged_from' = $2`, survivor.UUID, retired.UUID.String()); n != 1 {
		t.Errorf("%d share rows on the survivor name the retired company, want 1", n)
	}
}

// Two live shares of one grantee become one: the higher access, and no expiry
// over any expiry. The survivor's history records the before and after.
func TestACompanyMergeFoldsTwoSharesOfOneGrantee(t *testing.T) {
	c := setupCarry(t)
	retired, survivor := c.companyNamed(t, "Fold Retired GmbH"), c.companyNamed(t, "Fold Survivor GmbH")
	soon := time.Now().Add(48 * time.Hour)
	c.share(t, "company", retired.UUID, c.e.Rep1, "write", nil)
	c.share(t, "company", survivor.UUID, c.e.Rep1, "read", &soon)

	c.mergeCompanies(t, retired, survivor)

	if n := c.grantsNaming(t, "company", retired.UUID); n != 0 {
		t.Errorf("%d grants still name the retired company", n)
	}
	if n := c.grantsNaming(t, "company", survivor.UUID); n != 1 {
		t.Errorf("the survivor holds %d grants, want the one folded grant", n)
	}
	if access, expires := c.grantOn(t, "company", survivor.UUID, c.e.Rep1); access != "write" || expires != nil {
		t.Errorf("the folded grant is %s until %v, want write with no expiry", access, expires)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log
		WHERE action = 'record_share' AND entity_type = 'company' AND entity_id = $1
		  AND before->>'access' = 'read' AND after->>'access' = 'write'
		  AND evidence->>'merged_from' = $2`, survivor.UUID, retired.UUID.String()); n != 1 {
		t.Errorf("%d share rows record the fold on the survivor, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log
		WHERE action = 'record_unshare' AND entity_type = 'company' AND entity_id = $1`, retired.UUID); n != 1 {
		t.Errorf("%d unshare rows on the retired company, want 1", n)
	}
}

// Between two dated shares the later date wins.
func TestAFoldKeepsTheLaterExpiry(t *testing.T) {
	c := setupCarry(t)
	retired, survivor := c.companyNamed(t, "Later Retired GmbH"), c.companyNamed(t, "Later Survivor GmbH")
	sooner, later := time.Now().Add(24*time.Hour), time.Now().Add(30*24*time.Hour)
	c.share(t, "company", retired.UUID, c.e.Rep1, "read", &later)
	c.share(t, "company", survivor.UUID, c.e.Rep1, "read", &sooner)

	c.mergeCompanies(t, retired, survivor)

	_, expires := c.grantOn(t, "company", survivor.UUID, c.e.Rep1)
	if expires == nil || !expires.Equal(later.Truncate(time.Microsecond)) {
		t.Errorf("the folded grant runs until %v, want the later %v", expires, later)
	}
}

// An expired share confers nothing, so folding it in lifts nothing. The
// survivor's own read share stands unchanged and is not rewritten.
func TestAnExpiredShareDoesNotLiftTheSurvivorsShare(t *testing.T) {
	c := setupCarry(t)
	retired, survivor := c.companyNamed(t, "Lapsed Retired GmbH"), c.companyNamed(t, "Lapsed Survivor GmbH")
	soon := time.Now().Add(time.Hour)
	c.share(t, "company", retired.UUID, c.e.Rep1, "write", &soon)
	c.share(t, "company", survivor.UUID, c.e.Rep1, "read", nil)
	// The writer refuses a past expiry, so the lapse is set after the fact.
	if _, err := c.e.Pool.Exec(context.Background(), `UPDATE record_grant SET expires_at = now() - interval '1 day'
		WHERE record_type = 'company' AND record_id = $1`, retired.UUID); err != nil {
		t.Fatalf("letting the retired share lapse: %v", err)
	}

	c.mergeCompanies(t, retired, survivor)

	if access, expires := c.grantOn(t, "company", survivor.UUID, c.e.Rep1); access != "read" || expires != nil {
		t.Errorf("the survivor's grant is %s until %v, want read with no expiry", access, expires)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log
		WHERE action = 'record_share' AND entity_type = 'company' AND entity_id = $1
		  AND evidence ? 'merged_from'`, survivor.UUID); n != 0 {
		t.Errorf("%d share rows record a change the merge did not make", n)
	}
}

// A contact merge and a lead merge carry their shares the same way.
func TestContactAndLeadMergesMoveTheirShares(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Share Retired", "share-retired@carry.test")
	survivor := c.contactAt(t, "Share Survivor", "share-survivor@carry.test")
	c.share(t, "contact", retired.UUID, c.e.Rep1, "read", nil)
	c.merge(t, retired, survivor)
	if access, _ := c.grantOn(t, "contact", survivor.UUID, c.e.Rep1); access != "read" {
		t.Errorf("the contact survivor's grant is %q, want read", access)
	}

	retiredLead, survivorLead := c.leadNamed(t, "Retired Lead"), c.leadNamed(t, "Survivor Lead")
	c.share(t, "lead", retiredLead.UUID, c.e.Rep1, "write", nil)
	if _, err := c.contacts.MergeLead(c.admin, retiredLead, survivorLead); err != nil {
		t.Fatalf("merging the leads: %v", err)
	}
	if access, _ := c.grantOn(t, "lead", survivorLead.UUID, c.e.Rep1); access != "write" {
		t.Errorf("the lead survivor's grant is %q, want write", access)
	}
	if n := c.grantsNaming(t, "lead", retiredLead.UUID); n != 0 {
		t.Errorf("%d grants still name the retired lead", n)
	}
}

func (c *carryEnv) leadNamed(t *testing.T, name string) ids.LeadID {
	t.Helper()
	created, _, err := c.contacts.CreateLead(c.admin, contacts.CreateLeadInput{FullName: &name, Source: "manual"})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(created.Id))
}

func TestAnUnwiredMergeRefusesWhenAShareWouldBeStranded(t *testing.T) {
	c := setupCarry(t)
	unwired := contacts.NewStore(c.e.DB()).WithStopCarrier(c.consent).WithSatelliteCarriers(c.consent, c.intros)
	retired, survivor := c.companyNamed(t, "Unwired Retired GmbH"), c.companyNamed(t, "Unwired Survivor GmbH")
	c.share(t, "company", retired.UUID, c.e.Rep1, "read", nil)

	_, err := unwired.MergeCompany(c.admin, retired, survivor, nil)
	if _, ok := errors.AsType[*contacts.SatelliteCarrierNotWiredError](err); !ok {
		t.Fatalf("the merge answered %v, want SatelliteCarrierNotWiredError", err)
	}
}
