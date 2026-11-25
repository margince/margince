// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARunCarriesTheContextTagAndLinkPolicyItWasStagedWith(t *testing.T) {
	tag := ids.NewV7()
	run := toContractImportRun(migration.Run{Mapping: &migration.RunMapping{
		Object: migration.ObjectContact, ContextTag: tag.String(), OnBrokenLink: string(crmcontracts.ImportOnBrokenLinkDropLink),
	}})
	if run.ContextTagId == nil || run.ContextTagId.String() != tag.String() {
		t.Errorf("context_tag_id = %v, want %s", run.ContextTagId, tag)
	}
	if run.OnBrokenLink == nil || *run.OnBrokenLink != crmcontracts.ImportOnBrokenLinkDropLink {
		t.Errorf("on_broken_link = %v, want drop_link", run.OnBrokenLink)
	}
	bare := toContractImportRun(migration.Run{Mapping: &migration.RunMapping{Object: migration.ObjectContact}})
	if bare.ContextTagId != nil || bare.OnBrokenLink != nil {
		t.Errorf("a run staged with neither answers context_tag_id %v and on_broken_link %v, want both absent", bare.ContextTagId, bare.OnBrokenLink)
	}
}

func TestARunSaysTheRowsItWroteOnlyOnceItHasWritten(t *testing.T) {
	report := &migration.Report{Objects: []migration.ObjectReport{{
		Object: migration.ObjectContact, MirrorCount: 4, WillCreate: 3, Created: 2,
		Skipped: []migration.SkippedRow{{ExternalID: "a"}, {ExternalID: "b"}},
	}}}
	mapping := &migration.RunMapping{Object: migration.ObjectContact}
	partOf := func(status string) crmcontracts.ImportRunPart {
		run := toContractImportRun(migration.Run{Status: status, Mapping: mapping, Report: report})
		return (*run.Parts)[0]
	}
	for _, status := range []string{migration.StatusComplete, migration.StatusFailed, migration.StatusUndoing, migration.StatusUndone} {
		part := partOf(status)
		if part.RowCount != 4 || part.RowsWritten == nil || *part.RowsWritten != 2 {
			t.Errorf("a %s run reads %d rows and wrote %v, want 4 read and the 2 it created", status, part.RowCount, part.RowsWritten)
		}
	}
	for _, status := range []string{migration.StatusAwaitingApproval, migration.StatusRunning, migration.StatusCancelled} {
		if part := partOf(status); part.RowsWritten != nil {
			t.Errorf("a %s run says it wrote %d rows, want none said before it has written", status, *part.RowsWritten)
		}
	}
}

func TestADraftSaysTheRowsItsUploadCounted(t *testing.T) {
	draft := toContractImportRun(migration.Run{
		Status: migration.StatusPending, Mapping: &migration.RunMapping{Object: migration.ObjectContact},
		UploadRows: new(7),
	})
	if draft.RowCount == nil || *draft.RowCount != 7 {
		t.Errorf("row_count = %v, want the 7 rows the upload counted", draft.RowCount)
	}
	if part := (*draft.Parts)[0]; part.RowCount != 0 || part.RowsWritten != nil {
		t.Errorf("an untested draft's part reads %d rows and wrote %v, want 0 and none", part.RowCount, part.RowsWritten)
	}
	if released := toContractImportRun(migration.Run{Status: migration.StatusComplete}); released.RowCount != nil {
		t.Errorf("a run whose profile is gone says row_count %d, want none", *released.RowCount)
	}
}
