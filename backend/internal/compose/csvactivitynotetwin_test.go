// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestANoteKeyNamesThePendingRecordsNotTheirSpelling(t *testing.T) {
	at := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)
	subject := "Fair recap"
	in := activities.LogActivityInput{Kind: string(crmcontracts.ActivityKindNote), OccurredAt: &at, Subject: &subject}
	run := migration.RunID{}
	one := noteTwinKey(run, in, []string{"contact:P-1"})
	if again := noteTwinKey(run, in, []string{"contact:P-1", "contact:P-1"}); again != one {
		t.Fatalf("a record named twice keys %q, want %q", again, one)
	}
	if other := noteTwinKey(run, in, []string{"contact:P-2"}); other == one {
		t.Fatal("two different pending records keyed alike")
	}
	if none := noteTwinKey(run, in, nil); none == one {
		t.Fatal("a note with a pending link keyed like one with none")
	}
}
