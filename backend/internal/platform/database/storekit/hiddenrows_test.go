// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"strings"
	"testing"
)

func TestAnUnboundedSectionWithholdsNothingAndReadsNothing(t *testing.T) {
	section := HiddenSection{From: "project p", Where: "p.archived_at IS NULL", Owner: "p.owner_id"}
	// A nil transaction: any statement issued here would panic.
	got, err := ReadHiddenRows(context.Background(), nil, section, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 0 || got.Holders == nil || len(got.Holders) != 0 {
		t.Errorf("hidden = %+v, want count 0 and an empty, non-nil holder list", got)
	}
}

func TestTheListReadsTheSectionsBaseUnderItsVisibility(t *testing.T) {
	section := HiddenSection{From: "project p", Where: "p.archived_at IS NULL", Entry: "p.id", Owner: "p.owner_id", Visible: "p.owner_id = $1"}
	got := section.ListSQL("p.id", "LEFT JOIN app_user u ON u.id = p.owner_id")
	want := "SELECT p.id FROM project p LEFT JOIN app_user u ON u.id = p.owner_id WHERE p.archived_at IS NULL AND (p.owner_id = $1)"
	if strings.Join(strings.Fields(got), " ") != want {
		t.Errorf("list = %q, want %q", got, want)
	}
}
