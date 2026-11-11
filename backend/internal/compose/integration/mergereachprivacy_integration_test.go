// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A unit table declared both for privacy and for reach. The merge moves the
// reach columns onto the survivor, so those rows do not refuse the merge. A
// privacy link the merge does not move still refuses it.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// unitNotePrivacy declares the unit note table's links for privacy, the same
// columns unitNote declares for reach.
var unitNotePrivacy = privacy.Table{
	Name: unitNote.Name,
	Links: []privacy.Link{
		{Column: "company_id", Record: privacy.Company},
		{Column: "contact_id", Record: privacy.Contact},
		{Column: "record_id", Record: privacy.Lead},
	},
	Retention: jurisdiction.CommercialCorrespondence,
}

func TestEveryMergeMovesALinkTheUnitDeclaresForPrivacyAndReach(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	withUnitNoteTable(t)
	e.Contacts.WithExtensions(extprivacy.Declared([]privacy.Table{unitNotePrivacy}))
	src, tgt := seedMergePair(t, e, "Both declared")
	retired, survivor := e.SeedContact(t, "Ida Keller", nil), e.SeedContact(t, "Ida Kellar", nil)
	loser, live := seedLead(t, e, "Ola Berg", "ola@berg.test", nil), seedLead(t, e, "Ola Bergh", "", nil)
	e.WsExec(t, `INSERT INTO ext.ext_reachtest_note (company_id, contact_id, record_type, record_id, body)
		VALUES ($1, NULL, NULL, NULL, 'a'), (NULL, $2, NULL, NULL, 'b'), (NULL, NULL, 'lead', $3, 'c')`,
		src, retired, loser)

	if _, err := e.Contacts.MergeCompany(admin, src, tgt, nil); err != nil {
		t.Fatalf("company merge: %v", err)
	}
	if _, err := e.Contacts.MergeContact(admin, ContactIDOf(retired), ContactIDOf(survivor), nil); err != nil {
		t.Fatalf("contact merge: %v", err)
	}
	if _, err := e.Contacts.MergeLead(admin, loser, live); err != nil {
		t.Fatalf("lead merge: %v", err)
	}
	for _, c := range []struct {
		body, naming string
		want         ids.UUID
	}{
		{"a", "company_id", tgt.UUID},
		{"b", "contact_id", survivor},
		{"c", "record_id", live.UUID},
	} {
		got := e.WsScalar(t, `SELECT `+c.naming+`::text FROM ext.ext_reachtest_note WHERE body = $1`, c.body)
		if got != c.want.String() {
			t.Errorf("note %q names %s %s, want the survivor %s", c.body, c.naming, got, c.want)
		}
	}
}

// The refusal counts only the rows the merge would leave behind: the probe
// note, which no reach declaration moves, and not the unit note, which one does.
func TestAMergeRefusalCountsOnlyTheRowsTheMergeWouldNotMove(t *testing.T) {
	e := Setup(t)
	withUnitNoteTable(t)
	probeUnit(t, "1.0.0")
	e.Contacts.WithExtensions(extprivacy.Declared([]privacy.Table{unitNotePrivacy, {
		Name:      "ext_probe_note",
		Links:     []privacy.Link{{Column: "company_id", Record: privacy.Company}},
		Retention: jurisdiction.CommercialCorrespondence,
	}}))
	src, tgt := seedMergePair(t, e, "Half declared")
	e.WsExec(t, `INSERT INTO ext.ext_reachtest_note (company_id, body) VALUES ($1, 'a')`, src)
	e.WsExec(t, `INSERT INTO ext.ext_probe_note (company_id) VALUES ($1)`, src)

	_, err := e.Contacts.MergeCompany(e.Admin(), src, tgt, nil)
	var refusal *contacts.DependentRecordsError
	if !errors.As(err, &refusal) {
		t.Fatalf("merge of a company a probe note depends on = %v, want the dependents refusal", err)
	}
	if refusal.Count != 1 {
		t.Errorf("the refusal counts %d dependents, want only the probe note", refusal.Count)
	}
}
