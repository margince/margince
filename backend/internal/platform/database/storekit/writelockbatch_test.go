// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var advisoryLockSQL = regexp.MustCompile("(?s)`(SELECT pg_advisory_xact_lock.*?)`")

func lockStatementIn(t *testing.T, file string) string {
	t.Helper()
	source, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	match := advisoryLockSQL.FindSubmatch(source)
	if match == nil {
		t.Fatalf("%s holds no advisory lock statement", file)
	}
	return strings.Join(strings.Fields(string(match[1])), " ")
}

// A second statement that differed by a character would take a different lock
// and serialize nothing, while looking like the same call.
func TestQueuedWriteIdentityLockIsTheSameLock(t *testing.T) {
	direct := lockStatementIn(t, "storekit.go")
	queued := lockStatementIn(t, "writelockbatch.go")
	if direct != queued {
		t.Fatalf("the queued lock statement %q differs from LockWriteIdentity's %q", queued, direct)
	}
}
