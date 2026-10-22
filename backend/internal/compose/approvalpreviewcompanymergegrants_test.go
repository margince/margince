// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyMergeRequiresEveryReferenceGrantBeforeReadingRows(t *testing.T) {
	objects := []string{"company", "deal", "project", "contract", "offer", "commission", "finance", "integrations"}
	for _, missing := range objects {
		t.Run(missing, func(t *testing.T) {
			grants := make(map[string]principal.ObjectGrant)
			for _, object := range objects {
				grants[object] = principal.ObjectGrant{Read: object != missing}
			}
			actor := principal.Principal{Type: principal.PrincipalHuman, UserID: ids.NewV7(), Permissions: principal.Permissions{Objects: grants}}
			ctx := principal.WithActor(context.Background(), actor)
			// No database is present: a denied object must be refused before inspecting any rows.
			_, err := (approvalPreviewReader{}).companyMergeReferences(ctx, ids.NewV7())
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("missing %s read: %v", missing, err)
			}
		})
	}
}
