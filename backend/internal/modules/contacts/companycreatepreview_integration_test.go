// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyCreationPreviewMatchesDomainsOwnerAndAddressQueue(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	queued := 0
	e.store.WithGeocodeEnqueue(func(context.Context, pgx.Tx, ids.CompanyID) error { queued++; return nil })
	in := CreateCompanyInput{DisplayName: "Preview Buyer", Source: "manual", Domains: []CompanyDomainInput{{Domain: "WWW.PREVIEW-BUYER.TEST"}}, Address: &crmcontracts.Address{City: new("Berlin")}, LinkedInURL: new("http://www.linkedin.com/company/preview-buyer/about/")}
	preview, err := e.store.PreviewCompanyCreationForActor(ctx, in, actor)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 0 || !preview.Geocode || len(preview.Input.Domains) != 1 || preview.Input.Domains[0].Domain != "preview-buyer.test" || !preview.Input.Domains[0].IsPrimary {
		t.Fatalf("preview effects: %+v, queued=%d", preview, queued)
	}
	if in.Domains[0].Domain != "WWW.PREVIEW-BUYER.TEST" {
		t.Fatal("preview mutated the request")
	}
	created, err := e.store.CreateCompany(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 || created.OwnerId == nil || preview.Input.OwnerID == nil || ids.UUID(*created.OwnerId) != preview.Input.OwnerID.UUID {
		t.Fatalf("writer owner/queue differs: %+v,%+v,%d", created, preview, queued)
	}
	if created.Domains == nil || len(*created.Domains) != 1 || (*created.Domains)[0].Domain != preview.Input.Domains[0].Domain || !(*created.Domains)[0].IsPrimary {
		t.Fatalf("writer domains differ: %+v", created.Domains)
	}
	if created.LinkedinUrl == nil || preview.Input.LinkedInURL == nil || *created.LinkedinUrl != *preview.Input.LinkedInURL {
		t.Fatal("LinkedIn normalization differs")
	}
	_, err = e.store.PreviewCompanyCreationForActor(ctx, CreateCompanyInput{DisplayName: "Different name", Source: "manual", LinkedInURL: in.LinkedInURL}, actor)
	if _, ok := errors.AsType[*DuplicateCompanyLinkedInError](err); !ok {
		t.Fatalf("LinkedIn collision: %v", err)
	}
}

func TestCompanyCreationPreviewCanNameAReadOnlyReviewCandidate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	existing, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Preview Systems GmbH", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))})
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.RowScope = principal.RowScopeOwn
	ctx = principal.WithActor(ctx, actor)
	id := ids.From[ids.CompanyKind](ids.UUID(existing.Id))
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureWritable(ctx, tx, "company", id.UUID)
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("candidate unexpectedly writable: %v", err)
	}
	in := CreateCompanyInput{DisplayName: "Preview Systems Inc", Source: "manual"}
	preview, err := e.store.PreviewCompanyCreationForActor(ctx, in, actor)
	if err != nil || preview.ReviewCompanyID == nil || *preview.ReviewCompanyID != id {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	created, err := e.store.CreateCompany(ctx, in)
	if err != nil || created.Id == existing.Id {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	current, err := e.store.GetCompany(ctx, id, storekit.LiveOnly)
	if err != nil || current.DisplayName != existing.DisplayName || current.OwnerId == nil || ids.UUID(*current.OwnerId) != e.otherRep {
		t.Fatalf("review candidate changed: %+v err=%v", current, err)
	}
}
