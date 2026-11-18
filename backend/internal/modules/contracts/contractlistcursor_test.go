// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheContractListReadsBackItsOwnNextCursor(t *testing.T) {
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	token, err := termCursor(crmcontracts.Contract{Id: openapi_types.UUID(ids.NewV7()), CreatedAt: &created})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storekit.DecodeCursor(contractCursorScope, token); err != nil {
		t.Fatalf("the list refused its own cursor: %v", err)
	}
}
