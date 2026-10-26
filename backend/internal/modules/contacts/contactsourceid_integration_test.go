// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestTheSameOutsideIDCannotNameTwoContacts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	create := func(name string) error {
		_, err := e.store.CreateContact(ctx, CreateContactInput{
			FullName: name, Source: "manual", SourceSystem: new("contact-erp"), SourceID: new("P-1"),
		})
		return err
	}
	if err := create("Anna Müller"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := create("Berta Schmidt"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("second contact with the same pair = %v, want a conflict", err)
	}
}
