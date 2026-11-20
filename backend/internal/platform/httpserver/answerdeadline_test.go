// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
)

// A COMMIT under a request may run into the grace, and must be over by the
// write deadline. The request then still answers rather than closing its socket.
func TestARequestsCommitEndsByItsWriteDeadline(t *testing.T) {
	ctx, cancel := WithinResponseDeadline(context.Background(), time.Minute)
	defer cancel()

	by, stated := database.CommitDeadline(ctx)
	if !stated {
		t.Fatal("the request states no commit deadline, so a COMMIT may outlive its answer")
	}
	handlerBy, _ := ctx.Deadline()
	if past := by.Sub(handlerBy); past > AnswerGrace || past < AnswerGrace-time.Second {
		t.Errorf("the commit deadline is %s past the handler's, want the grace %s", past, AnswerGrace)
	}
}
