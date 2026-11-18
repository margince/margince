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

// BackfillTuning sizes one history page and its fetching. A zero field keeps
// the built-in value; the lookahead never falls below the workers.
type BackfillTuning struct {
	PageSize, FetchWorkers, Lookahead int
}

// WithBackfillTuning returns a copy that walks history pages by t.
func (c *Connector) WithBackfillTuning(t BackfillTuning) *Connector {
	copied := *c
	copied.tuning = t
	return &copied
}

// New returns a Gmail connector over the given OAuth + API surfaces.
func New(oauth googleconn.Authorizer, api API) *Connector {
	return &Connector{oauth: timedAuthorizer{oauth}, api: api, rawMemory: processRawMemory}
}
