// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// WithBounceSink returns a copy that records delivery reports instead of
// only dropping them.
func (c *Connector) WithBounceSink(sink connector.BounceSink) *Connector {
	copied := *c
	copied.bounces = sink
	return &copied
}

// New returns a Gmail connector over the given OAuth + API surfaces.
func New(oauth googleconn.Authorizer, api API) *Connector {
	return &Connector{oauth: timedAuthorizer{oauth}, api: api, rawMemory: processRawMemory}
}
