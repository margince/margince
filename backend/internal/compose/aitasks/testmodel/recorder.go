// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package testmodel records model-boundary requests for invocation tests.
package testmodel

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Recorder captures requests at the model boundary, even when a site's validator rejects its reply.
// Replies, when set, answers the calls in order; a call past them gets an empty structured reply.
type Recorder struct {
	Requests []model.Request
	Replies  []string
}

// Complete records the request and returns the next reply.
func (r *Recorder) Complete(_ context.Context, req model.Request) (model.Response, error) {
	r.Requests = append(r.Requests, req)
	if len(r.Requests) <= len(r.Replies) {
		return model.Response{Text: r.Replies[len(r.Requests)-1]}, nil
	}
	return model.Response{Text: "{}"}, nil
}
