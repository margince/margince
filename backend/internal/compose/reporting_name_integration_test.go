// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The contract bounds a report name in characters, so an accented name that
// fits is not refused for its byte length, and one a character too long is.
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
