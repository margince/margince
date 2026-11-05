// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestReviewedImportRefusesChangedSourceBeforeLanding(t *testing.T) {
	at := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		alter   func(*importReviewSource)
		refused bool
	}{
		{name: "identical source"},
		{name: "changed field", refused: true, alter: func(s *importReviewSource) {
			s.rows[migration.ObjectCompany][0].Fields["display_name"] = "Changed company"
		}},
		{name: "changed owner", refused: true, alter: func(s *importReviewSource) {
			s.rows[migration.ObjectCompany][0].OwnerExternalID = "another-owner"
		}},
		{name: "changed source date", refused: true, alter: func(s *importReviewSource) {
			s.rows[migration.ObjectCompany][0].LastSyncedAt = at.Add(time.Hour)
		}},
		{name: "missing row", refused: true, alter: func(s *importReviewSource) {
			s.rows[migration.ObjectCompany] = nil
		}},
		{name: "duplicate row", refused: true, alter: func(s *importReviewSource) {
			s.rows[migration.ObjectCompany] = append(s.rows[migration.ObjectCompany], s.rows[migration.ObjectCompany][0])
		}},
		{name: "missing part", refused: true, alter: func(s *importReviewSource) {
			s.objects = nil
		}},
		{name: "duplicate part", refused: true, alter: func(s *importReviewSource) {
			s.objects = append(s.objects, migration.ObjectCompany)
		}},
		{name: "extra association", refused: true, alter: func(s *importReviewSource) {
			s.associations = append(s.associations, migration.Assoc{FromType: migration.ObjectContact, FromID: "contact-one", ToType: migration.ObjectCompany, ToID: "company-one"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := migration.ReviewPlan{Version: 1, Complete: true, Enforced: true, Rows: []migration.ReviewRow{{
				Object: migration.ObjectCompany, ExternalID: "company-one", Outcome: "create", OwnerExternalID: "original-owner",
				LastSyncedAt: at, Values: map[string]json.RawMessage{"display_name": json.RawMessage(`"Original company"`)},
			}}}
			mapping := migration.RunMapping{Parts: []migration.PartMapping{{Object: migration.ObjectCompany}}}
			source, err := importSourceFromReview(plan, mapping)
			if err != nil {
				t.Fatal(err)
			}
			writers := &bundleWriters{parts: map[string]*csvWriters{migration.ObjectCompany: {}}}
			if err := writers.bindReviewPlan(&migration.Report{ReviewPlan: &plan}); err != nil {
				t.Fatal(err)
			}
			if tc.alter != nil {
				tc.alter(&source)
			}
			err = writers.validateReviewSource(context.Background(), source)
			if tc.refused && !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("changed source error = %v, want version skew", err)
			}
			if !tc.refused && err != nil {
				t.Fatalf("unchanged source refused: %v", err)
			}
		})
	}
}
