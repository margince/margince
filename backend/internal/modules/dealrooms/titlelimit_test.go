// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A title longer than its column is the caller's to shorten. It is refused at
// the field before any write, not by the database after one.
func TestARoomTitleOverItsLimitIsRefusedAtTheField(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("t", values.TextLabelCharacters+1)

	_, createErr := createInput(crmcontracts.CreateDealRoomRequest{
		DealId: openapi_types.UUID{1}, Title: long, Source: "manual",
	})
	_, updateErr := updateInput(crmcontracts.UpdateDealRoomRequest{Title: &long}, nil, nil)

	for name, err := range map[string]error{"create": createErr, "update": updateErr} {
		t.Run(name, func(t *testing.T) { faulttest.AssertNamesField(t, err, columnTitle) })
	}
}

func TestARoomTitleAtItsLimitIsAccepted(t *testing.T) {
	t.Parallel()
	title := strings.Repeat("t", values.TextLabelCharacters)
	if _, err := updateInput(crmcontracts.UpdateDealRoomRequest{Title: &title}, nil, nil); err != nil {
		t.Fatalf("a title of exactly the limit was refused: %v", err)
	}
}
