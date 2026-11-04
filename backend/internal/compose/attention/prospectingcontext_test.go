// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestProspectingPagesFindLeadWorkBeyondTheGeneralFirstPage(t *testing.T) {
	tasks := &stubTasks{}
	for i := range 30 {
		tasks.rows = append(tasks.rows, Task{ID: ids.NewV7(), Subject: fmt.Sprintf("Review offer %d", i), LinkType: "deal", LinkID: ids.NewV7()})
	}
	for i := range 3 {
		tasks.rows = append(tasks.rows, Task{ID: ids.NewV7(), Subject: fmt.Sprintf("Call prospect %d", i), LinkType: "lead", LinkID: ids.NewV7()})
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	first, err := svc.Worklist(pageReader(), "", string(crmcontracts.WorklistFilterProspecting), ids.UUID{}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Queue) != 2 || first.Summary.Total != 3 || first.NextCursor == nil {
		t.Fatalf("first page: %+v", first)
	}
	second, err := svc.Worklist(pageReader(), "", string(crmcontracts.WorklistFilterProspecting), ids.UUID{}, 2, *first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Queue) != 1 || second.Summary.Total != 3 || second.NextCursor != nil {
		t.Fatalf("second page: %+v", second)
	}
	for _, row := range append(first.Queue, second.Queue...) {
		if !buildsPipeline(row) {
			t.Fatalf("unrelated work: %+v", row)
		}
	}
}
