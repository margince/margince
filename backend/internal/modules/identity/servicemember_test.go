// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A service member acts with nobody at the keyboard, so only a literal
// installation admin who is a human may mint or retire one. Every refusal here
// happens before the database is touched.
func TestOnlyAHumanInstallationAdminMayDeclareOrArchiveAServiceMember(t *testing.T) {
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: ids.NewV7()})
	agent := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, OnBehalfOf: ids.NewV7()})
	admin := Identity{Roles: []string{roleAdmin}}
	rep := Identity{Roles: []string{"rep"}}

	cases := []struct {
		name  string
		ctx   context.Context
		actor Identity
		want  error
	}{
		{"an agent holding the admin role is refused", agent, admin, apperrors.ErrPermissionDenied},
		{"a human without the admin role is refused", human, rep, apperrors.ErrPermissionDenied},
	}
	svc := &Service{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := requireInstallationAdmin(c.ctx, c.actor); !errors.Is(err, c.want) {
				t.Errorf("requireInstallationAdmin = %v, want %v", err, c.want)
			}
			if _, err := svc.DeclareServiceMember(c.ctx, c.actor, ServiceMemberInput{Extension: "x", DisplayName: "X"}); !errors.Is(err, c.want) {
				t.Errorf("DeclareServiceMember = %v, want %v", err, c.want)
			}
			if err := svc.ArchiveServiceMember(c.ctx, c.actor, ids.UserID{}); !errors.Is(err, c.want) {
				t.Errorf("ArchiveServiceMember = %v, want %v", err, c.want)
			}
		})
	}
	if err := requireInstallationAdmin(human, admin); err != nil {
		t.Errorf("a human admin was refused: %v", err)
	}
}

func TestTheServiceMemberRoutesRefuseTheUnauthenticatedTheMalformedAndTheNonAdmin(t *testing.T) {
	long := strings.Repeat("x", 300)
	post := func(ctx context.Context, h func(http.ResponseWriter, *http.Request), body string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/users/service-members", strings.NewReader(body)).WithContext(ctx)
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec.Code
	}
	h := Handlers{svc: &Service{}}
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: ids.NewV7()})
	asRep := withIdentity(human, Identity{Roles: []string{"rep"}})

	archive := func(ctx context.Context) int {
		rec := httptest.NewRecorder()
		h.ArchiveServiceMember(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx), crmcontracts.Id(ids.NewV7()))
		return rec.Code
	}
	if got := archive(context.Background()); got != http.StatusUnauthorized {
		t.Errorf("archive unauthenticated = %d", got)
	}
	if got := archive(asRep); got != http.StatusForbidden {
		t.Errorf("archive by a non-admin = %d", got)
	}
	for name, c := range map[string]struct {
		ctx  context.Context
		body string
		want int
	}{
		"unauthenticated":   {context.Background(), `{}`, http.StatusUnauthorized},
		"malformed body":    {asRep, `{`, http.StatusUnprocessableEntity},
		"blank extension":   {asRep, `{"extension":"  ","display_name":"X"}`, http.StatusUnprocessableEntity},
		"long extension":    {asRep, `{"extension":"` + long + `","display_name":"X"}`, http.StatusUnprocessableEntity},
		"blank name":        {asRep, `{"extension":"billing","display_name":" "}`, http.StatusUnprocessableEntity},
		"long name":         {asRep, `{"extension":"billing","display_name":"` + long + `"}`, http.StatusUnprocessableEntity},
		"a non-admin human": {asRep, `{"extension":"billing","display_name":"Feed"}`, http.StatusForbidden},
	} {
		if got := post(c.ctx, h.DeclareServiceMember, c.body); got != c.want {
			t.Errorf("%s: status %d, want %d", name, got, c.want)
		}
	}
}
