// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (p *Provider) PreviewMergeSurvivorship(ctx context.Context, kind string, source, target ids.UUID, sourceVersion, targetVersion *int64) (MergeSurvivorship, error) {
	return p.store.PreviewMergeSurvivorship(ctx, kind, source, target, sourceVersion, targetVersion)
}
