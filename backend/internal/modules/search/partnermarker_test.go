// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The marker rides a company hit and nothing else, and a company the reader
// checked and found no programme for is marked false rather than left unknown.
func TestThePartnerMarkerIsTakenOnCompanyHitsAlone(t *testing.T) {
	t.Parallel()
	partner, plain, contact := ids.NewV7(), ids.NewV7(), ids.NewV7()
	var asked []ids.CompanyID
	store := &Store{partnerMarks: func(_ context.Context, _ pgx.Tx, companyIDs []ids.CompanyID) (map[ids.CompanyID]bool, error) {
		asked = companyIDs
		return map[ids.CompanyID]bool{ids.From[ids.CompanyKind](partner): true}, nil
	}}
	hits := []Hit{
		{Type: hitTypeCompany, ID: partner},
		{Type: hitTypeCompany, ID: plain},
		{Type: "contact", ID: contact},
	}

	if err := store.markPartners(context.Background(), nil, hits); err != nil {
		t.Fatal(err)
	}

	if hits[0].IsPartner == nil || !*hits[0].IsPartner {
		t.Errorf("the company with a live programme is marked %v, want true", hits[0].IsPartner)
	}
	if hits[1].IsPartner == nil || *hits[1].IsPartner {
		t.Errorf("a company with no programme is marked %v, want a literal false — absent from the "+
			"reader's answer is a checked no, not an unknown", hits[1].IsPartner)
	}
	if hits[2].IsPartner != nil {
		t.Errorf("a contact hit carries a partner marker (%v); a partner is a property of a company",
			*hits[2].IsPartner)
	}
	if len(asked) != 2 {
		t.Errorf("the reader was asked about %d id(s), want the two company hits alone", len(asked))
	}
}

// A seat that may not read partner programmes still gets its search, unmarked.
func TestASeatWithoutThePartnerGrantGetsAnUnmarkedPage(t *testing.T) {
	t.Parallel()
	store := &Store{partnerMarks: func(context.Context, pgx.Tx, []ids.CompanyID) (map[ids.CompanyID]bool, error) {
		return nil, apperrors.ErrPermissionDenied
	}}
	hits := []Hit{{Type: hitTypeCompany, ID: ids.NewV7()}}

	if err := store.markPartners(context.Background(), nil, hits); err != nil {
		t.Fatalf("a refused marker failed the whole search: %v", err)
	}
	if hits[0].IsPartner != nil {
		t.Errorf("a caller who may not read partner programmes was told %v", *hits[0].IsPartner)
	}
}

// A read that BROKE is not a refusal: it fails the search rather than serving a
// page with partner accounts rendered as ordinary ones and nothing saying so.
func TestAFailedPartnerReadFailsTheSearch(t *testing.T) {
	t.Parallel()
	refused := errors.New("connection reset")
	store := &Store{partnerMarks: func(context.Context, pgx.Tx, []ids.CompanyID) (map[ids.CompanyID]bool, error) {
		return nil, refused
	}}
	hits := []Hit{{Type: hitTypeCompany, ID: ids.NewV7()}}

	err := store.markPartners(context.Background(), nil, hits)

	if !errors.Is(err, refused) {
		t.Fatalf("a broken partner read → %v, want the failure to reach the caller", err)
	}
	if hits[0].IsPartner != nil {
		t.Error("a failed read still left a marker on the page")
	}
}

// Nothing bound a reader — a worker's store, a test that does not ask.
func TestAStoreWithNoPartnerReaderLeavesEveryMarkerUnset(t *testing.T) {
	t.Parallel()
	hits := []Hit{{Type: hitTypeCompany, ID: ids.NewV7()}}

	if err := (&Store{}).markPartners(context.Background(), nil, hits); err != nil {
		t.Fatal(err)
	}
	if hits[0].IsPartner != nil {
		t.Errorf("an unbound store marked a company hit %v", *hits[0].IsPartner)
	}
}
