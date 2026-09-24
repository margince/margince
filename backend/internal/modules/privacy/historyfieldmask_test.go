// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maskedReader binds a row-scoped human carrying the given masks. RowScopeTeam
// and not All: an unbounded seat skips masks outright, so a fixture on All
// would assert nothing about what one withholds.
func maskedReader(masks ...principal.FieldMask) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeTeam,
			FieldMasks: masks,
		},
	})
}

func alwaysMask(object, field string) principal.FieldMask {
	return principal.FieldMask{Object: object, Field: field, Condition: principal.MaskAlways}
}

func conditionedMask(object, field string) principal.FieldMask {
	return principal.FieldMask{Object: object, Field: field, Condition: principal.MaskOutsideWriteAuthority}
}

// Every case here passes a NIL transaction, which is the assertion under the
// assertion: a reader whose masks need no write arm must not reach the database
// at all, and one that did would panic rather than quietly cost a statement.
func TestTheMaskAReaderReadsARecordsHistoryUnder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		reader     context.Context
		entityType string
		want       []string
	}{
		{
			name:       "a reader no mask reaches withholds nothing",
			reader:     maskedReader(),
			entityType: "deal",
			want:       nil,
		},
		{
			name:       "a mask on the amount takes its whole money group",
			reader:     maskedReader(alwaysMask("deal", "amount_minor")),
			entityType: "deal",
			want:       []string{"amount_minor", "currency", "expected_arr_minor"},
		},
		{
			name:       "a mask on another object says nothing about this one",
			reader:     maskedReader(alwaysMask("deal", "amount_minor")),
			entityType: contactObject,
			want:       nil,
		},
		{
			// The invariant auth owns and this module must not respell: write
			// authority is a question an activity's rows cannot answer, so the
			// condition never lifts there and no write arm is resolved. Delete
			// shareableObject from auth's conditionLifts and this fails.
			name:       "a conditioned mask withholds where the condition has no answer",
			reader:     maskedReader(conditionedMask(entityTypeActivity, "body")),
			entityType: entityTypeActivity,
			want:       []string{"body"},
		},
		{
			// The partner is a FACET of the company: its fields are written
			// into the company's audit images, so the company's trail is where
			// a mask on the partner has to reach or the tier is readable in
			// full from every past value of it.
			name:       "a facet's mask reaches the trail its fields are written into",
			reader:     maskedReader(alwaysMask("partner", "margin_tier")),
			entityType: "company",
			want:       []string{"margin_tier"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mask, err := maskForRecord(tc.reader, nil, tc.entityType, ids.NewV7())
			if err != nil {
				t.Fatalf("resolving the mask: %v", err)
			}
			got := make([]string, 0, len(mask))
			for field := range mask {
				got = append(got, field)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("withheld = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnUnauthenticatedReaderResolvesNoMaskAtAll(t *testing.T) {
	t.Parallel()
	_, err := maskForRecord(context.Background(), nil, "deal", ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("err = %v, want permission denied — a mask is a question about a principal", err)
	}
}

func TestAFieldFilterIsRefusedExactlyWhenTheFieldIsWithheld(t *testing.T) {
	t.Parallel()
	mask := entityFieldMask{"amount_minor": {}}
	readable, withheld := "name", "amount_minor"
	for _, field := range []*string{nil, &readable} {
		if err := refuseMaskedFieldFilter(field, mask, "deal"); err != nil {
			t.Errorf("filtering by a field this reader reads → %v, want it allowed", err)
		}
	}

	err := refuseMaskedFieldFilter(&withheld, mask, "deal")
	var refused *values.ParseError
	if !errors.As(err, &refused) {
		t.Fatalf("filtering by a withheld field → %v, want a client refusal", err)
	}
	if refused.Code != auth.CodeFieldMasked || refused.Field != "field" {
		t.Errorf("refusal = %s on %q, want %s on field", refused.Code, refused.Field, auth.CodeFieldMasked)
	}
	// It names the field it refused. A refusal a client cannot attribute to a
	// control reads as the whole request being malformed.
	if !strings.Contains(refused.Message, withheld) {
		t.Errorf("the refusal reads %q, want it to name %s", refused.Message, withheld)
	}
}
