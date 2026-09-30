// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A container somebody MADE is theirs. A container the PROVIDER defines means
// the same thing everywhere, so a rule over one may bind the installation.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// The point of the widening: an installation can say it captures no
// promotions, once, for everybody.
func TestAProviderCategoryMayBindTheWholeInstallation(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewExclusionStore(InstallationDB(e.Pool))

	for _, value := range capture.ProviderDefinedContainers() {
		if _, err := store.Add(e.Admin(), capture.ExclusionScopeWorkspace,
			capture.ExclusionKindContainer, value); err != nil {
			t.Fatalf("adding %s at workspace scope: %v", value, err)
		}
	}
}

// The reasoning the original constraint was written for still holds: a label
// somebody made lives in one mailbox, so a workspace rule over it would bind
// every colleague's connection to a place that does not exist there.
func TestALabelSomebodyMadeStillCannotBindEverybody(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewExclusionStore(InstallationDB(e.Pool))

	_, err := store.Add(e.Admin(), capture.ExclusionScopeWorkspace,
		capture.ExclusionKindContainer, "gmail:Label_7")
	if err == nil {
		t.Fatal("a user-made label was accepted at workspace scope")
	}
}

// And the refusal is a stated one rather than a constraint violation reaching
// a reader as a 500: the writer checks before the database does.
func TestTheRefusalNamesTheScopeRatherThanFailingOnTheConstraint(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewExclusionStore(InstallationDB(e.Pool))

	_, err := store.Add(e.Admin(), capture.ExclusionScopeWorkspace,
		capture.ExclusionKindContainer, "graph:AAMk-private")
	var invalid *capture.InvalidExclusionError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want a stated refusal naming the field", err)
	}
	if invalid.Field != "scope" {
		t.Fatalf("refusal names %q, want the scope", invalid.Field)
	}
}

// A provider category is still writable as somebody's own rule — widening the
// scope it MAY take does not take away the one it had.
func TestAProviderCategoryIsStillWritableAsAPersonalRule(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewExclusionStore(InstallationDB(e.Pool))

	if _, err := store.Add(e.As(e.Rep1, nil, integration.AccountRepPerms),
		capture.ExclusionScopeUser, capture.ExclusionKindContainer,
		capture.GmailCategoryPromotions); err != nil {
		t.Fatalf("adding a category as a personal rule: %v", err)
	}
}
