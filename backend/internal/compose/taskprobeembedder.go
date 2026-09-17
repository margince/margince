// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The embed lane a DB-less debug loop ranks with.
//
// Its own file rather than beside the chat probes in sitereaddebug.go because
// it is the opposite half of the model path: those build a completer for one
// task, this builds the retrieval embedder every task shares, and the only
// thing the two have in common is the --model spelling they both read.

import (
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/vectorkit"
)

// TaskProbeEmbedder builds the embed lane a DB-less debug loop ranks with, from
// the same provider:model override the chat lanes take.
//
// It is the PRODUCT's embed lane — ai.Router, which is exactly what
// compose.NewModelPath hands the knowledge store as its Embedder — rather than
// a client the debug loop drives itself. A second embed path would embed the
// question differently from the way the corpus was embedded, and the whole
// value of ranking against a real binding is that the two agree.
//
// Unlike TaskProbeBrain this binds ONLY the embeddings slot: the tiers stay
// empty because nothing here completes anything, and a chat binding invented
// alongside would suggest this lane can spend money on one.
//
//nolint:ireturn // vectorkit.Embedder IS the seam the knowledge store takes; returning *ai.Router would hand a caller the whole router.
func TaskProbeEmbedder(modelSpec string) (vectorkit.Embedder, string, error) {
	binding, err := parseModelSpec(modelSpec)
	if err != nil {
		return nil, "", err
	}
	cfg := ai.RoutingConfig{
		Profile:    ai.ProfileCloudFrontier,
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: binding},
	}
	// Bound to the environment for the reason pinnedModelRouting binds it: with
	// no lookup a cloud binding fails closed on a missing BYOK key while the key
	// sits in the environment, unread.
	router, err := ai.NewLocalRouter(cfg.WithKeys(config.FromOS), ai.WithoutResultCache())
	if err != nil {
		return nil, "", err
	}
	return router, "embed lane " + modelSpec, nil
}
