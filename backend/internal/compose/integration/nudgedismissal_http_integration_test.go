// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func readNudgeState(t *testing.T, e *apptest.AppEnv, contactID string) crmcontracts.RelationshipNudgeState {
	t.Helper()
	var got crmcontracts.RelationshipNudgeState
	mustCall(t, e, "GET", "/v1/contacts/"+contactID+"/nudge-dismissal", nil, http.StatusOK, &got)
	return got
}

type createdContact struct {
	ID string `json:"id"`
}

func createContact(t *testing.T, e *apptest.AppEnv, body AnyMap) createdContact {
	t.Helper()
	var created createdContact
	mustCall(t, e, "POST", "/v1/contacts", body, http.StatusCreated, &created)
	return created
}

func countDismissals(t *testing.T, e *apptest.AppEnv, contactID string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM relationship_nudge_dismissal WHERE contact_id = $1`, contactID).Scan(&n); err != nil {
		t.Fatalf("counting dismissals of %s: %v", contactID, err)
	}
	return n
}

func TestANudgeDismissalGoesFromSetAsideToStoppedAndBackToFollowing(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Nudge journey", "journey@fable.test", "Admin")
	contact := createContact(t, e, AnyMap{"full_name": "Gone Quiet", "source": "manual"})
	path := "/v1/contacts/" + contact.ID + "/nudge-dismissal"

	setAside := func(days int) {
		t.Helper()
		mustCall(t, e, "PUT", path, AnyMap{"days": days}, http.StatusNoContent, nil)
		got := readNudgeState(t, e, contact.ID)
		if got.State != crmcontracts.RelationshipNudgeStateStateSetAside || got.DismissedUntil == nil {
			t.Fatalf("after %d days aside the state = %+v, want set_aside with a date", days, got)
		}
		var setAt time.Time
		if err := e.Owner.QueryRow(context.Background(),
			`SELECT set_at FROM relationship_nudge_dismissal WHERE contact_id = $1`, contact.ID).Scan(&setAt); err != nil {
			t.Fatalf("reading when the dismissal was set: %v", err)
		}
		if want := setAt.AddDate(0, 0, days); !got.DismissedUntil.Equal(want) {
			t.Errorf("%d days aside ends %v, want %v: %d days after it was set at %v", days, got.DismissedUntil, want, days, setAt)
		}
	}
	setAside(30)
	setAside(60)

	mustCall(t, e, "PUT", path, AnyMap{"permanent": true}, http.StatusNoContent, nil)
	if got := readNudgeState(t, e, contact.ID); got.State != crmcontracts.RelationshipNudgeStateStateStopped || got.DismissedUntil != nil {
		t.Fatalf("after a permanent stop the state = %+v, want stopped with no date", got)
	}

	mustCall(t, e, "DELETE", path, nil, http.StatusNoContent, nil)
	if got := readNudgeState(t, e, contact.ID); got.State != crmcontracts.RelationshipNudgeStateStateFollowing || got.DismissedUntil != nil {
		t.Fatalf("after resuming the state = %+v, want following", got)
	}
	mustCall(t, e, "DELETE", path, nil, http.StatusNoContent, nil)
}

func TestAHiddenContactIsNotFoundThroughItsNudgePreference(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Nudge scope", "scope@fable.test", "Admin")
	e.DescribeCompany(t)
	colleague := inviteColleague(t, e)
	hidden := createContact(t, e, AnyMap{"full_name": "Not Yours", "source": "manual", "owner_id": colleague})
	setVisibility(t, e, hidden.ID, "owner")
	quiet := createContact(t, e, AnyMap{"full_name": "Already Quiet", "source": "manual", "owner_id": colleague})
	mustCall(t, e, "PUT", "/v1/contacts/"+quiet.ID+"/nudge-dismissal", AnyMap{"days": 30}, http.StatusNoContent, nil)
	if n := countDismissals(t, e, quiet.ID); n != 1 {
		t.Fatalf("the admin's dismissal left %d rows, want one", n)
	}
	setVisibility(t, e, quiet.ID, "owner")
	own := createContact(t, e, AnyMap{"full_name": "Yours", "source": "manual"})
	demoteToRep(t, e)
	mustCall(t, e, "GET", "/v1/contacts/"+hidden.ID, nil, http.StatusNotFound, nil)
	mustCall(t, e, "DELETE", "/v1/contacts/"+quiet.ID+"/nudge-dismissal", nil, http.StatusNotFound, nil)
	if n := countDismissals(t, e, quiet.ID); n != 1 {
		t.Errorf("a refused resume left %d dismissal rows on a hidden contact, want the one already there", n)
	}

	doors := []struct {
		method string
		body   any
		want   int
	}{
		{"GET", nil, http.StatusOK},
		{"PUT", AnyMap{"days": 30}, http.StatusNoContent},
		{"PUT", AnyMap{"permanent": true}, http.StatusNoContent},
		{"DELETE", nil, http.StatusNoContent},
	}
	for _, door := range doors {
		var hiddenProblem, missingProblem AnyMap
		if status := e.Call(t, door.method, "/v1/contacts/"+hidden.ID+"/nudge-dismissal", door.body, nil, &hiddenProblem); status != http.StatusNotFound {
			t.Errorf("%s %v on a hidden contact → %d, want 404", door.method, door.body, status)
		}
		mustCall(t, e, door.method, "/v1/contacts/"+ids.NewV7().String()+"/nudge-dismissal", door.body, http.StatusNotFound, &missingProblem)
		if hiddenProblem["code"] == nil || !reflect.DeepEqual(hiddenProblem, missingProblem) {
			t.Errorf("%s %v: a hidden contact answers %v and a missing one %v, want the same refusal", door.method, door.body, hiddenProblem, missingProblem)
		}
		if n := countDismissals(t, e, hidden.ID); n != 0 {
			t.Errorf("%s %v wrote %d dismissal rows for a contact the reader cannot see, want none", door.method, door.body, n)
		}
		mustCall(t, e, door.method, "/v1/contacts/"+own.ID+"/nudge-dismissal", door.body, door.want, nil)
		if door.method == "PUT" {
			if n := countDismissals(t, e, own.ID); n < 1 {
				t.Fatalf("%s %v on the rep's own contact left %d dismissal rows, want one: the count must see a write", door.method, door.body, n)
			}
		}
	}
}
