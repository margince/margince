// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
)

// The lawful basis a real send recorded against a contact follows that contact
// into the survivor of a merge, through the server's own wiring. The basis is
// the ground the mail stood on; left on the retired record, the survivor's
// subject-access export and every later reader lose it.
func TestTheBasisARealSendRecordedFollowsTheMerge(t *testing.T) {
	c := setupConsent(t)
	// A reply on a live deal the subject is staked on is a supported
	// resolution, which is the arm that writes communication_basis.
	c.stakeADeal(t)
	if status, code := c.send(t, "business_correspondence"); status != http.StatusAccepted && status != http.StatusOK {
		t.Fatalf("the reply was not sent: %d %s", status, code)
	}
	var recorded int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, c.contactID).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded == 0 {
		t.Fatal("the send recorded no lawful basis, so this fixture no longer reaches the writer it is about")
	}

	var survivor struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/contacts", AnyMap{
		"source":    "manual",
		"full_name": "Consent Survivor", "emails": []AnyMap{{"email": "survivor@consent.test"}},
	}, nil, &survivor); status != http.StatusCreated {
		t.Fatalf("create the survivor → %d", status)
	}
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/merge",
		AnyMap{"target_id": survivor.ID}, nil, nil); status != http.StatusOK {
		t.Fatalf("merge → %d", status)
	}

	var carried, left int
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE contact_id = $2), count(*) FILTER (WHERE contact_id = $1)
		  FROM communication_basis WHERE contact_id IN ($1, $2)`,
		c.contactID, survivor.ID).Scan(&carried, &left); err != nil {
		t.Fatal(err)
	}
	if carried != recorded || left != 0 {
		t.Fatalf("after the merge the survivor holds %d basis row(s) and the retired contact %d, want %d and 0",
			carried, left, recorded)
	}
}
