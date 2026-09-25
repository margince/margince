// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Both seat writers ask two questions in a fixed order — may the caller add a
// seat, then has the installation described itself — and refuse before touching
// the database. The service here holds no database, so a writer that reached one
// would panic rather than pass.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type seatWriter struct {
	name string
	add  func(*Service, Identity) error
}

var seatWriters = []seatWriter{
	{"invite", func(svc *Service, actor Identity) error {
		_, _, err := svc.InviteUser(actorCtx(context.Background(), actor), actor,
			InviteUserInput{Email: "new@acme.test", DisplayName: "New", Role: "rep"})
		return err
	}},
	{"former member", func(svc *Service, actor Identity) error {
		_, err := svc.CreateFormerMember(actorCtx(context.Background(), actor), actor,
			FormerMemberInput{Email: "gone@acme.test", DisplayName: "Gone"})
		return err
	}},
}

func seatActor(userAdmin principal.ObjectGrant) Identity {
	return Identity{
		UserID: ids.New[ids.UserKind](), SeatType: string(principal.SeatFull),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{objectUserAdmin: userAdmin},
			RowScope: principal.RowScopeAll,
		},
	}
}

func TestNoSeatIsAddedBeforeTheInstallationDescribesItself(t *testing.T) {
	svc := &Service{installationDescribed: func(context.Context) (bool, error) { return false, nil }}
	admin := seatActor(principal.ObjectGrant{Create: true, Read: true})
	for _, w := range seatWriters {
		if err := w.add(svc, admin); !errors.Is(err, errCompanyNotDescribed) {
			t.Errorf("%s before the company is described = %v, want errCompanyNotDescribed", w.name, err)
		}
	}
}

// RBAC first: a caller who may not add seats learns nothing about the company.
func TestACallerWhoMayNotAddSeatsIsRefusedBeforeTheCompanyIsAsked(t *testing.T) {
	for _, described := range []bool{false, true} {
		asked := false
		svc := &Service{installationDescribed: func(context.Context) (bool, error) {
			asked = true
			return described, nil
		}}
		rep := seatActor(principal.ObjectGrant{Read: true})
		for _, w := range seatWriters {
			if err := w.add(svc, rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("%s by a rep (described=%v) = %v, want ErrPermissionDenied", w.name, described, err)
			}
		}
		if asked {
			t.Errorf("described=%v: the company was asked before the caller's grant refused", described)
		}
	}
}

func TestAFailedAnswerIsNotReadAsUndescribed(t *testing.T) {
	outage := errors.New("database unavailable")
	svc := &Service{installationDescribed: func(context.Context) (bool, error) { return false, outage }}
	if err := svc.refuseUntilDescribed(context.Background()); !errors.Is(err, outage) || errors.Is(err, errCompanyNotDescribed) {
		t.Errorf("refusal on a failed read = %v, want the read's own error", err)
	}
}

// Unwired is a composition fault; sending the admin to describe a company they
// may already have saved would point them at the wrong fix.
func TestAnUnwiredAnswerIsAFaultRatherThanTheRefusal(t *testing.T) {
	err := (&Service{}).refuseUntilDescribed(context.Background())
	if err == nil || errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("unwired answer = %v, want a non-conflict fault", err)
	}
}

func TestTheRefusalRendersAsACoded409(t *testing.T) {
	var refusal *httperr.DetailedError
	if !errors.As(companyNotDescribedRefusal(errCompanyNotDescribed), &refusal) {
		t.Fatal("errCompanyNotDescribed was not rendered as a refusal")
	}
	if refusal.Status != http.StatusConflict || refusal.Code != "company_not_described" {
		t.Errorf("refusal = %d/%q, want 409/company_not_described", refusal.Status, refusal.Code)
	}
	if got := companyNotDescribedRefusal(errEmailTaken); !errors.Is(got, errEmailTaken) {
		t.Errorf("an unrelated conflict was rewritten into %v", got)
	}
}
