// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"strings"
	"testing"
)

// A subject id that is not an id is reported, not skipped.
//
// The send gate locks its subject before writing the ground it relied on. A
// subject whose id cannot be parsed is a wiring fault, and taking no lock
// would leave the write unprotected while the caller heard nothing. The parse
// fails before any statement runs, so this needs no transaction.
func TestASubjectIdThatIsNotAnIdIsReported(t *testing.T) {
	t.Parallel()
	err := lockSendSubject(context.Background(), nil, subjectRef{
		Kind: entityContact, ID: "not-an-id",
	})
	if err == nil {
		t.Fatal("a send subject with an unparseable id locked nothing and reported nothing")
	}
	if !strings.Contains(err.Error(), "not an id") {
		t.Errorf("the error reads %q, want it to name the id as the fault", err)
	}
}
