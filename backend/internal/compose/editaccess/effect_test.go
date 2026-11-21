// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package editaccess

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The share is given as the owner the request names. Anyone else deciding it
// is refused before the decision is spent.
func TestOnlyTheNamedOwnerRunsTheShare(t *testing.T) {
	owner, other := ids.NewV7(), ids.NewV7()
	body, err := json.Marshal(approvalsubject.EditAccess{RecordType: "lead", RecordID: ids.NewV7(), OwnerID: owner, RequesterID: ids.NewV7()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: other})
	err = acceptEffect(nil, nil)(ctx, ids.ApprovalID{UUID: ids.NewV7()}, body, "")
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member who is not the owner ran the share: %v, want permission denied", err)
	}
}
