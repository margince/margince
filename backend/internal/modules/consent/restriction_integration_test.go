// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Recording a restriction of processing (GDPR Art. 18) against a real
// database: through a privacy request, and through the contact's stop door.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *dsrEnv) seedContact(t *testing.T) ids.ContactID {
	t.Helper()
	contact := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Anna Weber', 'manual', 'human:x')`,
		contact); err != nil {
		t.Fatal(err)
	}
	return contact
}

// asActor is e.ctx with the principal changed by edit.
func (e *dsrEnv) asActor(t *testing.T, edit func(*principal.Principal)) context.Context {
	t.Helper()
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the harness context carries no principal")
	}
	edit(&actor)
	return principal.WithActor(e.ctx, actor)
}

func (e *dsrEnv) restrictions(t *testing.T, contact ids.ContactID) (rows int, level, through string) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*), coalesce(min(decided_by_level), ''), coalesce(min(recorded_through), '')
		  FROM communication_suppression
		 WHERE contact_id = $1 AND kind = 'processing_restriction' AND revoked_at IS NULL`,
		contact).Scan(&rows, &level, &through); err != nil {
		t.Fatal(err)
	}
	return rows, level, through
}

func (e *dsrEnv) fulfilOverHTTP(t *testing.T, ctx context.Context, request ids.UUID) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandlers(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	body := `{"status":"fulfilled","resolution":"restricted while the address is disputed","identity_check":"id_document"}`
	r := httptest.NewRequest(http.MethodPatch, "/v1/data-subject-requests/"+request.String(),
		strings.NewReader(body)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.UpdateDataSubjectRequest(w, r, openapi_types.UUID(request))
	return w
}

func TestFulfillingARestrictionRequestRecordsOneRestriction(t *testing.T) {
	e := setupDSR(t)
	contact := e.seedContact(t)
	req := e.mustCreate(t, dsrKindRestriction, contact.String())

	if w := e.fulfilOverHTTP(t, e.ctx, req.ID); w.Code != http.StatusOK {
		t.Fatalf("fulfilling a restriction request: %d %s", w.Code, w.Body)
	}
	// A replayed fulfil answers the closed request and records nothing more.
	if w := e.fulfilOverHTTP(t, e.ctx, req.ID); w.Code != http.StatusOK {
		t.Fatalf("replaying the fulfilment: %d %s", w.Code, w.Body)
	}

	rows, level, through := e.restrictions(t, contact)
	if rows != 1 || level != "subject" || through != "colleague" {
		t.Fatalf("want one subject-level restriction a colleague recorded, got %d at %q through %q", rows, level, through)
	}
	var audits, events int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1 AND after->>'suppression_kind' = 'processing_restriction'`,
		contact).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'consent.suppressed'`,
	).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || events != 1 {
		t.Fatalf("want one audit row and one event for the restriction, got %d and %d", audits, events)
	}
	after, err := e.store.GetDSR(e.ctx, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != dsrStatusFulfilled {
		t.Fatalf("the request is %q after its restriction was recorded", after.Status)
	}
}

func TestOnlyAnAdminInTheAppFulfilsARestriction(t *testing.T) {
	e := setupDSR(t)
	refused := map[string]context.Context{
		"an agent": e.asActor(t, func(p *principal.Principal) {
			p.Type, p.ID, p.OnBehalfOf = principal.PrincipalAgent, "agent:"+ids.NewV7().String(), e.user
		}),
		"a colleague who is not an admin": e.asActor(t, func(p *principal.Principal) { p.Permissions.RoleKeys = []string{"ops"} }),
	}
	for name, ctx := range refused {
		t.Run(name, func(t *testing.T) {
			contact := e.seedContact(t)
			req := e.mustCreate(t, dsrKindRestriction, contact.String())
			if w := e.fulfilOverHTTP(t, ctx, req.ID); w.Code != http.StatusForbidden {
				t.Fatalf("want 403, got %d %s", w.Code, w.Body)
			}
			if rows, _, _ := e.restrictions(t, contact); rows != 0 {
				t.Fatalf("a refused fulfilment recorded %d restriction(s)", rows)
			}
		})
	}
}

func TestARestrictionRequestIsNotClosedByHand(t *testing.T) {
	e := setupDSR(t)
	req := e.mustCreate(t, dsrKindRestriction, e.seedContact(t).String())
	_, err := e.store.UpdateDSR(e.ctx, req.ID, UpdateDSRInput{
		Status: new(dsrStatusFulfilled), Resolution: new("done"), Proof: idDocumentChecked,
	})
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != fieldStatus {
		t.Fatalf("closing a restriction request without recording it answered %v", err)
	}
}

func TestARestrictionRequestNamingNoContactIsRefused(t *testing.T) {
	e := setupDSR(t)
	req := e.mustCreate(t, dsrKindRestriction, "anna.weber@brandt-automotive.de")
	w := e.fulfilOverHTTP(t, e.ctx, req.ID)
	if w.Code != http.StatusUnprocessableEntity || validationField(t, w.Body.Bytes()) != fieldSubjectRef {
		t.Fatalf("want 422 on subject_ref, got %d %s", w.Code, w.Body)
	}
}

func TestTheStopDoorTakesARestrictionFromAnAdminOnly(t *testing.T) {
	e := setupDSR(t)
	contact := e.seedContact(t)
	in := SuppressInput{ContactID: contact, Kind: kindProcessingRestriction}

	refused := map[string]context.Context{
		"a rep": boundedRepCtx(e.ws, e.user),
		"an agent with admin grants": e.asActor(t, func(p *principal.Principal) {
			p.Type, p.ID, p.OnBehalfOf = principal.PrincipalAgent, "agent:"+ids.NewV7().String(), e.user
		}),
		"the system": e.asActor(t, func(p *principal.Principal) {
			p.Type, p.ID = principal.PrincipalSystem, "system:retention"
		}),
	}
	for name, ctx := range refused {
		if err := e.store.Suppress(ctx, in); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s recording a restriction answered %v, want ErrPermissionDenied", name, err)
		}
	}
	if rows, _, _ := e.restrictions(t, contact); rows != 0 {
		t.Fatalf("a refused stop recorded %d restriction(s)", rows)
	}
	if err := e.store.Suppress(e.ctx, in); err != nil {
		t.Fatalf("an admin recording a restriction: %v", err)
	}
	if rows, level, _ := e.restrictions(t, contact); rows != 1 || level != "subject" {
		t.Fatalf("want one subject-level restriction, got %d at %q", rows, level)
	}
}
