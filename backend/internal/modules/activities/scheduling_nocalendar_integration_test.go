// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A host with no calendar is told to connect one: a request of theirs, which
// the tool door reads from the same store error as the REST door. Reading
// another user's calendar stays a permission denial.
func TestReliableAvailabilityWithoutACalendarAsksForOne(t *testing.T) {
	for name, withoutService := range map[string]bool{"no provider chosen": false, "no calendar service": true} {
		f := newInvitationFixture(t)
		store := f.store
		if withoutService {
			store = store.WithSchedulingCalendar(nil)
		} else {
			profile := defaultSchedulingProfile()
			profile.Provider = ""
			if _, err := store.SaveSchedulingProfile(f.ctx, profile); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		host, _ := principal.Actor(f.ctx)
		_, _, err := store.ReliableAvailability(f.ctx, ids.From[ids.UserKind](host.UserID), monday(8), monday(18), 0)
		fault, ok := errors.AsType[apperrors.FieldFault](err)
		if !ok {
			t.Fatalf("%s: store answered %v, want a field refusal both doors classify as 422", name, err)
		}
		if field, _, message := fault.FieldFault(); field != "provider" || !strings.Contains(message, "Connect a calendar") {
			t.Errorf("%s: refusal names %q %q, want the provider and how to connect one", name, field, message)
		}

		reliable := true
		rec := httptest.NewRecorder()
		Handlers{store: store}.GetAvailability(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx),
			crmcontracts.GetAvailabilityParams{From: monday(8), To: monday(18), Reliable: &reliable})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: REST answered %d %s, want 422", name, rec.Code, rec.Body.String())
		}
	}
}

func TestReliableAvailabilityOfAnotherUsersCalendarStaysForbidden(t *testing.T) {
	f := newInvitationFixture(t)
	other := openapi_types.UUID(ids.NewV7())
	reliable := true
	rec := httptest.NewRecorder()
	Handlers{store: f.store}.GetAvailability(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx),
		crmcontracts.GetAvailabilityParams{From: monday(8), To: monday(18), Reliable: &reliable, HostUserId: &other})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a colleague's calendar answered %d %s, want 403", rec.Code, rec.Body.String())
	}
}
