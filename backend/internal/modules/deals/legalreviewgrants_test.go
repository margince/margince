// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
)

// The answer, the reviewer list and the inbox card name one object. The answer
// spells it as a literal for the table-ownership gate, so this holds the two.
func TestTheAnswerAndTheReviewerListNameOneObject(t *testing.T) {
	if legalReviewEntity != approvalsubject.ObjectLegalReview {
		t.Errorf("deals answers under %q, the reviewer list and card under %q", legalReviewEntity, approvalsubject.ObjectLegalReview)
	}
}
