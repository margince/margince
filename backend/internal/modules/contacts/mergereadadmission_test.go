// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMergeLinksRequireEveryKindBeforeReadingRows(t *testing.T) {
	for _, denied := range []string{"activity", "list", "tag"} {
		t.Run(denied, func(t *testing.T) {
			grants := map[string]principal.ObjectGrant{"activity": {Read: true}, "list": {Read: true}, "tag": {Read: true}}
			delete(grants, denied)
			ctx := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: grants}})
			_, err := previewMergeLinks(ctx, nil, contactEntity, ids.UUID{}, ids.UUID{})
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("missing %s read grant: %v", denied, err)
			}
		})
	}
}

func TestMergeHandoffsRequireReadBeforeReadingRows(t *testing.T) {
	ctx := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalHuman})
	_, err := previewMergeHandoffs(ctx, nil, contactEntity, ids.UUID{})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("missing handoff read grant: %v", err)
	}
}
