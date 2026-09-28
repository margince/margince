// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The reserved prefix is machinery for the importer's replay key. A reader must
// never see it: the timeline renders author.via straight out of the column, so
// an unstripped row reads "Logged in mirror:hubspot by …".

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheAuthorViaDropsTheImporterPrefix(t *testing.T) {
	stored := "mirror:hubspot"
	name := "Mutaz Suleiman"
	id := ids.NewV7()

	author := SourceAuthorOf(&id, nil, &name, &stored)
	if author == nil {
		t.Fatal("a row naming an author must answer one")
	}
	if author.Via == nil || *author.Via != "hubspot" {
		t.Errorf("via = %v, want hubspot — the mirror: prefix is not a reader's business", author.Via)
	}

	// An ordinary source system is passed through untouched, or the stripping
	// would be eating characters off names that never carried the prefix.
	plain := "hubspot"
	if author := SourceAuthorOf(&id, nil, &name, &plain); author.Via == nil || *author.Via != "hubspot" {
		t.Errorf("via = %v, want an ordinary source system unchanged", author.Via)
	}
	// An unrecorded origin stays absent rather than becoming an empty string.
	if author := SourceAuthorOf(&id, nil, &name, nil); author.Via != nil {
		t.Errorf("via = %v, want nil when the row records no origin", author.Via)
	}
}
