// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// An omitted required id is refused BY NAME, not read as a zero.
//
// Both ids on this body are required and neither is a pointer, so a caller who
// leaves one out sends nothing and the decoder produces the zero UUID without
// complaint. Unguarded, that zero would be compared against the deal's real
// closing and answer "this deal has closed again since the review was started"
// — a confident, specific, wrong explanation for a field the caller simply
// forgot.

import (
	"context"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRequiredReviewBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()
	reviews := &OutcomeReviews{}

	for _, c := range []struct {
		name string
		req  crmcontracts.CreateOutcomeReviewRequest
		want string
	}{
		{
			name: "no closing named",
			req: crmcontracts.CreateOutcomeReviewRequest{
				SubmissionId: openapi_types.UUID(ids.NewV7()),
			},
			want: "closing_occurrence_id",
		},
		{
			name: "no submission id",
			req: crmcontracts.CreateOutcomeReviewRequest{
				ClosingOccurrenceId: openapi_types.UUID(ids.NewV7()),
			},
			want: "submission_id",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// No pool and no transaction: the guard runs before either is
			// reached, which is the point — a body this incomplete never gets
			// as far as a database.
			_, err := reviews.Write(context.Background(), ids.New[ids.DealKind](), c.req)
			if err == nil {
				t.Fatalf("an omitted %s was accepted", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the refusal does not name %s: %v", c.want, err)
			}
		})
	}
}
