// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The inherited project is locked in the same call as the deal it came with,
// so no attach probe holds it first.
func TestAnInheritedProjectIsLockedWithItsDeal(t *testing.T) {
	deal, project := ids.NewV7(), ids.NewV7()

	got := dealRelinkTargets(deal, project)
	if !slices.Equal(got.Deals, []ids.UUID{deal}) || !slices.Equal(got.Projects, []ids.UUID{project}) {
		t.Errorf("a relink onto the deal locks %+v, want the deal and its project", got)
	}
	if got := dealRelinkTargets(deal, ids.Nil); len(got.Projects) != 0 {
		t.Errorf("a deal with no project locks projects %v, want none", got.Projects)
	}
}

// A copied link set relinks its own project before any deal could bring one.
func TestACopiedLinkSetFilesItsOwnProjectFirst(t *testing.T) {
	deal := ActivityLinkInput{EntityType: linkEntityDeal, EntityID: ids.NewV7()}
	contact := ActivityLinkInput{EntityType: linkEntityContact, EntityID: ids.NewV7()}
	project := ActivityLinkInput{EntityType: linkEntityProject, EntityID: ids.NewV7()}

	got := projectLinksFirst([]ActivityLinkInput{deal, contact, project})
	if want := []ActivityLinkInput{project, deal, contact}; !slices.Equal(got, want) {
		t.Errorf("the copy order is %v, want %v", got, want)
	}
}
