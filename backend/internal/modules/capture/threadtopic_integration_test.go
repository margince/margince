// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"testing"
)

// The SQL spelling of a thread topic answers every case as the Go one does. The
// thread join and the reply's own boundary then never disagree on a topic.
func TestTheThreadTopicSQLAgreesWithItsGoSpelling(t *testing.T) {
	owner := ownerConn(t)
	for _, c := range threadTopicCases {
		var got string
		if err := owner.QueryRow(context.Background(), `SELECT `+threadTopicSQL("$1::text"), c.subject).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != threadTopic(c.subject) {
			t.Errorf("threadTopicSQL(%q) = %q, Go reads %q", c.subject, got, threadTopic(c.subject))
		}
	}
}
