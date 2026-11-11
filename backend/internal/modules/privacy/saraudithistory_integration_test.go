// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRelatedAuditHistoryProvesEachImageBelongsToTheSubject(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, other, record := ids.NewV7(), ids.NewV7(), ids.NewV7()
	image := func(contact ids.UUID, label string) map[string]any {
		return map[string]any{
			"linked_record_type": "contact", "linked_record_id": contact,
			"label": label, "unknown_private_field": "Other private evidence",
		}
	}
	for _, change := range [][2]map[string]any{
		{nil, {"plan_id": ids.NewV7(), "label": "Unproven earlier label"}},
		{image(other, "Earlier other contact's label"), image(subject, "Subject's new label")},
		{image(subject, "Subject's previous label"), image(other, "Later other contact's label")},
	} {
		action := "update"
		if change[0] == nil {
			action = "create"
		}
		if _, err := storekit.AuditWithEvidence(ctx, tx, action, "weekly_plan_commitment", record,
			change[0], change[1], map[string]any{"private_detail": "Other private evidence"}); err != nil {
			t.Fatal(err)
		}
	}
	var pkg SARPackage
	section := sarAuditHistorySection(&pkg, []ids.UUID{subject}, nil)
	rows, err := rowMaps(ctx, tx, section.query, section.args...)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("related audit rows=%d, want only two provably bound changes", len(rows))
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, withheld := range []string{"Unproven earlier label", "other contact's label", "Other private evidence"} {
		if strings.Contains(string(encoded), withheld) {
			t.Fatalf("export disclosed %q: %s", withheld, encoded)
		}
	}
	for _, own := range []string{"Subject's new label", "Subject's previous label"} {
		if !strings.Contains(string(encoded), own) {
			t.Fatalf("export omitted %q: %s", own, encoded)
		}
	}
}
