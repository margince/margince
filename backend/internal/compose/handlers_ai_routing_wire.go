// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// routingToWire and routingFromWire carry a binding's broker preferences. The
// pointer is the meaning: nil is "the product default" and an empty struct is
// "no preferences", so neither direction may turn one into the other.
//
// Both go through JSON because the contract type IS OpenRouter's request
// shape, whose sort and thresholds are each a string-or-object union.
func routingToWire(r *ai.OpenRouterRouting) (*crmcontracts.AiOpenRouterRouting, error) {
	if r == nil {
		return nil, nil
	}
	raw, err := r.RequestJSON()
	if err != nil {
		return nil, err
	}
	var out crmcontracts.AiOpenRouterRouting
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// routingFromWire reads the routing value at path. raw is the value exactly as
// the client sent it, so a key the contract type would drop is refused by its
// path; without it the decoded contract value is read.
func routingFromWire(path string, r *crmcontracts.AiOpenRouterRouting, raw json.RawMessage) (*ai.OpenRouterRouting, error) {
	if r == nil {
		return nil, nil
	}
	if raw == nil {
		var err error
		if raw, err = json.Marshal(r); err != nil {
			return nil, err
		}
	}
	return ai.DecodeRouting(path, raw)
}

// sentRouting is each lane's routing value as the client wrote it, keyed by
// its path in the document, read from the body Decode kept.
func sentRouting(r *http.Request) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	if raw, ok := httperr.PresentField(r, "tiers"); ok {
		var tiers map[string]struct {
			Routing json.RawMessage `json:"routing"`
		}
		if json.Unmarshal(raw, &tiers) == nil {
			for name, tier := range tiers {
				if tier.Routing != nil {
					out[ai.TierRoutingPath(ai.Tier(name))] = tier.Routing
				}
			}
		}
	}
	if raw, ok := httperr.PresentField(r, "embeddings"); ok {
		var lane struct {
			Routing json.RawMessage `json:"routing"`
		}
		if json.Unmarshal(raw, &lane) == nil && lane.Routing != nil {
			out[ai.EmbeddingsRoutingPath] = lane.Routing
		}
	}
	return out
}
