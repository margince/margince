// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package testmodel records model-boundary requests for invocation tests.
package testmodel

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Recorder captures requests at the model boundary, even when a site's validator rejects its reply.
type Recorder struct {
	Requests []model.Request
}

// Complete records the request and returns an empty structured reply.
func (r *Recorder) Complete(_ context.Context, req model.Request) (model.Response, error) {
	r.Requests = append(r.Requests, req)
	return model.Response{Text: "{}"}, nil
}
