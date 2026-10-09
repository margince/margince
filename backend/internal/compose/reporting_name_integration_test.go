// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The contract bounds a report name in characters. An accented name that fits
// is accepted, and one a character too long is refused.
func TestAReportNameLimitCountsCharactersNotBytes(t *testing.T) {
	f := reportingBusiness(t)
	create := func(name string) error {
		_, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{
			Name: name, Audience: "private", Selection: f.selection(),
		})
		return err
	}
	if err := create(strings.Repeat("é", 160)); err != nil {
		t.Fatalf("160 characters (320 bytes) refused: %v", err)
	}
	if err := create(strings.Repeat("é", 161)); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("161 characters answered %v, want an invalid-argument refusal", err)
	}
	if err := create("\u200b"); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("an invisible name answered %v, want an invalid-argument refusal", err)
	}
}

// A target's revision reason is bounded in characters, like the contract says.
func TestATargetReasonLimitCountsCharactersNotBytes(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	ctx := reportingActor(e)
	create := func(month time.Month, reason string) error {
		_, err := service.CreateTarget(ctx, crmcontracts.ReportingTargetInput{
			Metric: "bookings_won", Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)},
			PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC)},
			Value: 1000, Reason: reason,
		})
		return err
	}
	if err := create(time.October, strings.Repeat("é", 1000)); err != nil {
		t.Fatalf("1000 characters (2000 bytes) refused: %v", err)
	}
	if err := create(time.November, strings.Repeat("é", 1001)); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("1001 characters answered %v, want an invalid-argument refusal", err)
	}
}
