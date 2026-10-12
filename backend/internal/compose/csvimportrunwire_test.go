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
