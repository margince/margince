// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/filterpropose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The company context behind a proposal is read only on request, so a caller
// the handler did not opt in — one without company read — never reaches the
// read that would refuse them, and the proposal goes ahead without it.
func TestAProposalReadsTheCompanyContextOnlyWhenAskedTo(t *testing.T) {
	reader := &contextReaderStub{err: apperrors.ErrPermissionDenied}
	provider := newCompanyContextProvider(reader)
	req := filterpropose.Request(filterpropose.Input{
		Resource: "contact", Text: "contacts in Germany", Today: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Lang: "en",
	})

	if _, err := provider.Prepare(context.Background(), ai.TaskNlSearch, req); err != nil {
		t.Fatalf("a proposal not opted in failed on the company context: %v", err)
	}
	if len(reader.calls) != 0 {
		t.Fatalf("the company context was read for a proposal that did not ask for it: %v", reader.calls)
	}

	req.IncludeCompanyContext = true
	if _, err := provider.Prepare(context.Background(), ai.TaskNlSearch, req); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an opted-in read did not reach the reader: err=%v calls=%v", err, reader.calls)
	}
}
