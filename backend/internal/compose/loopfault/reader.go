// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package loopfault supplies a read-only failure boundary for worker certification.
package loopfault

import (
	"context"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Failure may refuse a read; it cannot return records or authorize a write.
type Failure func(context.Context, datasource.EntityRef) error

type reader struct {
	datasource.SystemOfRecordProvider
	fail Failure
}

// Wrap preserves every real writer and the registry's decoder and permission gate.
// A nil failure leaves the provider itself in place.
func Wrap(provider datasource.SystemOfRecordProvider, fail Failure) datasource.SystemOfRecordProvider {
	if fail == nil {
		return provider
	}
	return reader{SystemOfRecordProvider: provider, fail: fail}
}

func (r reader) Read(ctx context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	if err := r.fail(ctx, ref); err != nil {
		return datasource.Record{}, err
	}
	return r.SystemOfRecordProvider.Read(ctx, ref)
}
