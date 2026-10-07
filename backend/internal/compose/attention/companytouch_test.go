// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The company pass: a row about an account says which side wrote last.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubCompanyTouch struct {
	moments map[ids.UUID]TouchMoments
	asked   [][]ids.UUID
	err     error
}

func (s *stubCompanyTouch) LastTouch(_ context.Context, companyIDs []ids.UUID) (map[ids.UUID]TouchMoments, error) {
	s.asked = append(s.asked, append([]ids.UUID(nil), companyIDs...))
	return s.moments, s.err
}

func companyRow(company ids.UUID) crmcontracts.WorklistItem {
	return crmcontracts.WorklistItem{
		Id:      ids.NewV7().String(),
		Source:  "deal_suggestion",
		Subject: &crmcontracts.AttentionSubject{Type: subjectCompany, Id: openapi_types.UUID(company)},
	}
}

// Every company on the page is asked about in one call, each row gets its own
// two moments, and a company the reader may not see keeps its id and gains none.
func TestCompanyRowsSayWhenEachSideLastWrote(t *testing.T) {
	acme, hidden := ids.NewV7(), ids.NewV7()
	weWrote := rankInstant.Add(-9 * 24 * time.Hour)
	touch := &stubCompanyTouch{moments: map[ids.UUID]TouchMoments{acme: {LastOutbound: &weWrote}}}
	svc := (&Service{}).WithCompanyTouch(touch)
	rows := []crmcontracts.WorklistItem{
		companyRow(acme), companyRow(acme), companyRow(hidden),
		{Id: "deal", Source: "deal_at_risk", Subject: &crmcontracts.AttentionSubject{Type: subjectDeal, Id: openapi_types.UUID(ids.NewV7())}},
	}

	if err := svc.nameTheCompanies(context.Background(), rows); err != nil {
		t.Fatalf("naming the companies: %v", err)
	}

	if want := [][]ids.UUID{{acme, hidden}}; !slices.EqualFunc(touch.asked, want, slices.Equal) {
		t.Fatalf("the reader was asked %v, wanted one call naming %v, each once, as first met", touch.asked, want)
	}
	for _, row := range rows[:2] {
		got := row.Company
		if got == nil || ids.UUID(got.Id) != acme || got.Touch == nil || got.Touch.LastInboundAt != nil ||
			got.Touch.LastOutboundAt == nil || !got.Touch.LastOutboundAt.Equal(weWrote) {
			t.Fatalf("Acme's row says %+v, wanted we wrote at %s and they never did", got, weWrote)
		}
	}
	if rows[2].Company == nil || rows[2].Company.Touch != nil {
		t.Fatalf("the hidden company's row carries %+v, wanted the id and no moments", rows[2].Company)
	}
	if rows[3].Company != nil {
		t.Fatalf("a deal row names the company %+v, wanted none", rows[3].Company)
	}
}

// A reader who may not read activity is refused the moments, not the page.
func TestARefusedActivityReadWithholdsTheAccountMomentsNotTheRows(t *testing.T) {
	svc := (&Service{}).WithCompanyTouch(&stubCompanyTouch{err: apperrors.ErrPermissionDenied})
	rows := []crmcontracts.WorklistItem{companyRow(ids.NewV7())}
	if err := svc.nameTheCompanies(context.Background(), rows); err != nil {
		t.Fatalf("a refusal failed the page: %v", err)
	}
	if rows[0].Company == nil || rows[0].Company.Touch != nil {
		t.Fatalf("the row carries %+v, wanted its company and no moments", rows[0].Company)
	}
	broken := (&Service{}).WithCompanyTouch(&stubCompanyTouch{err: errors.New("connection reset")})
	if err := broken.nameTheCompanies(context.Background(), rows); err == nil {
		t.Fatal("a database failure was rendered as an account nobody wrote to")
	}
}

// Unbound, a company row still names its company, and claims no moments.
func TestAnUnboundReaderStillNamesTheCompany(t *testing.T) {
	company := ids.NewV7()
	rows := []crmcontracts.WorklistItem{companyRow(company)}
	if err := (&Service{}).nameTheCompanies(context.Background(), rows); err != nil {
		t.Fatalf("an unbound pass failed: %v", err)
	}
	if rows[0].Company == nil || ids.UUID(rows[0].Company.Id) != company || rows[0].Company.Touch != nil {
		t.Fatalf("an unbound pass carried %+v, wanted the company and no moments", rows[0].Company)
	}
}
