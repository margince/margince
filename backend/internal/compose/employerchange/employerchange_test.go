// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package employerchange

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
)

// A proposer wired without an inbox refuses, rather than staging nowhere or
// panicking inside the capture that called it.
func TestAProposerWithoutAnInboxRefuses(t *testing.T) {
	err := Proposer{}.ProposeEmployerChangesTx(context.Background(), nil, []approvalsubject.EmployerChange{{}})
	if !errors.Is(err, errNoInbox) {
		t.Fatalf("an unwired proposer answered %v, want errNoInbox", err)
	}
}
