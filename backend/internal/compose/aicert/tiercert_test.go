// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// The committed ladder verdict table is what the committed records give.
func TestTheTierCertTableIsCurrent(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the census: %v", err)
	}
	corpus, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	records, err := LoadRecords("records")
	if err != nil {
		t.Fatalf("loading the records: %v", err)
	}
	rows, err := TierCertTable(context.Background(), corpus, census, records)
	if err != nil {
		t.Fatalf("building the table: %v", err)
	}
	want, err := RenderTierCertTable(rows)
	if err != nil {
		t.Fatalf("rendering the table: %v", err)
	}
	committed, err := os.ReadFile(filepath.Join("..", "..", "modules", "ai", "tiercert_gen.go"))
	if err != nil {
		t.Fatalf("reading the committed table: %v", err)
	}
	if !bytes.Equal(committed, want) {
		t.Fatal("the ladder verdict table is not what the committed records give — run `make gen`")
	}
}
