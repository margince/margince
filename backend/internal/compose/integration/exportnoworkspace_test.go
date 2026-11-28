// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnExportWithAnActorButNoWorkspaceIsRefusedBeforeAnyRead(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String()})
	var dst bytes.Buffer

	_, err := compose.NewExportWriter(nil).WriteBundle(ctx, &dst)

	if !errors.Is(err, database.ErrNoWorkspace) {
		t.Fatalf("WriteBundle = %v, want database.ErrNoWorkspace", err)
	}
	if dst.Len() != 0 {
		t.Fatalf("the refused export wrote %d bytes", dst.Len())
	}
}
