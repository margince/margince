// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (p *Provider) PreviewMergeSurvivorship(ctx context.Context, kind string, source, target ids.UUID, sourceVersion, targetVersion *int64) (contacts.MergeSurvivorship, error) {
	return p.contacts.PreviewMergeSurvivorship(ctx, kind, source, target, sourceVersion, targetVersion)
}
