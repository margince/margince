// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func AcceptedInquiryOwner(raw *map[string]any) (*ids.UUID, error) {
	return workflow.AcceptedInquiryOwner(raw)
}
