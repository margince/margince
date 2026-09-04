// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fakeNames answers a fixed name set, and records what it was asked for.
// The seam it stands in for is a real boundary — every real implementation
// reads the database under the caller's grants — so the labeller's own
// behaviour is what these cases are about.
type fakeNames struct {
	labels map[ids.UUID]string
	err    error
	asked  []ids.UUID
	entity string
}

func (f *fakeNames) Labels(_ context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error) {
	f.entity = entityType
	f.asked = append(f.asked, want...)
	if f.err != nil {
		return nil, f.err
	}
	return f.labels, nil
}

func TestASourceRowIsNamedForTheReader(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	names := &fakeNames{labels: map[ids.UUID]string{id: "Contoso renewal"}}
	rows := []map[string]any{{"id": id.String(), "amount_base_minor": int64(500000)}}

	if !labelDerivationRows(context.Background(), names, "deal", rows) {
		t.Fatal("a row the seam named reported as unnamed")
	}
	if got := rows[0][derivationLabelColumn]; got != "Contoso renewal" {
		t.Errorf("row carries label %v, want the name the seam answered", got)
	}
	if names.entity != "deal" {
		t.Errorf("seam asked about %q, want the spec's own entity", names.entity)
	}
}

// The whole reason the drill-through can show a name at all: the seam holds
// the reader's grants. A row it declines to name keeps its id and loses only
// the label — the number it contributes to is unchanged.
func TestARowTheReaderMayNotNameKeepsItsID(t *testing.T) {
	t.Parallel()
	named, withheld := ids.NewV7(), ids.NewV7()
	names := &fakeNames{labels: map[ids.UUID]string{named: "Contoso renewal"}}
	rows := []map[string]any{
		{"id": named.String()},
		{"id": withheld.String()},
	}

	labelDerivationRows(context.Background(), names, "deal", rows)

	if _, has := rows[1][derivationLabelColumn]; has {
		t.Error("a row the seam withheld came back with a label")
	}
	if rows[1]["id"] != withheld.String() {
		t.Error("the withheld row lost its id, so it cannot be reconciled at all")
	}
}

// A seam that will not answer costs the labels and nothing else. Failing the
// explanation would hide a number the caller is entitled to see, and the
// arithmetic never depended on the names.
func TestALabelReadThatFailsLeavesTheRowsIntact(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	names := &fakeNames{err: errors.New("the label store is down")}
	rows := []map[string]any{{"id": id.String(), "amount_base_minor": int64(500000)}}

	if labelDerivationRows(context.Background(), names, "deal", rows) {
		t.Error("a failed label read reported rows as named")
	}
	if rows[0]["amount_base_minor"] != int64(500000) {
		t.Error("a failed label read disturbed the row's measures")
	}
}

// The agent surface leaves the seam unwired, and answers in ids.
func TestWithoutANamesSeamTheRowsAreUnlabelled(t *testing.T) {
	t.Parallel()
	rows := []map[string]any{{"id": ids.NewV7().String()}}

	if labelDerivationRows(context.Background(), nil, "deal", rows) {
		t.Error("an unwired seam reported rows as named")
	}
	if _, has := rows[0][derivationLabelColumn]; has {
		t.Error("an unwired seam produced a label")
	}
}

// One read per type is the seam's whole shape, so two rows standing for the
// same record must not become two questions.
func TestTheSeamIsAskedAboutEachRecordOnce(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	names := &fakeNames{labels: map[ids.UUID]string{id: "Contoso renewal"}}
	rows := []map[string]any{{"id": id.String()}, {"id": id.String()}}

	labelDerivationRows(context.Background(), names, "deal", rows)

	if len(names.asked) != 1 {
		t.Errorf("seam asked about %d ids, want 1 for two rows of one record", len(names.asked))
	}
	if rows[1][derivationLabelColumn] != "Contoso renewal" {
		t.Error("the second row of the same record went unnamed")
	}
}
