// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// WithReadRecordFailure inserts a refusal before read_record's real provider read.
// Every other tool retains the provider, including its optional write interfaces.
func WithReadRecordFailure(fail func(context.Context, datasource.EntityRef) error) RegistryOption {
	return func(r *Registry) { r.readRecordFailure = fail }
}
