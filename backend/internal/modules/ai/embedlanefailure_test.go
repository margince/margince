// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The embed lane has no ladder, so no walk ends for it and ErrAllTiersFailed
// never marks its outage. It carries a sentinel of its own, or a handler waiting
// on it cannot tell a provider being down from its own fault.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAnEmbedLaneThatDidNotAnswerIsNamed(t *testing.T) {
	down := errors.New("ai: gemini: http 503")
	r := assembleRouter(map[Tier]model.Client{}, erringEmbedder{err: down}, ProfileEUHosted, &memMeter{},
		DefaultMonthlyTokens, nil, nil, false, nil)
	_, err := r.Embed(wsContext(t), model.EmbedRequest{Inputs: []string{"q"}})
	if !errors.Is(err, ErrEmbedLaneFailed) {
		t.Errorf("an embed provider failure read as %v, want ErrEmbedLaneFailed", err)
	}
	if !errors.Is(err, down) {
		t.Errorf("the provider's own cause was lost: %v", err)
	}
}

// erringEmbedder fails every embedding.
type erringEmbedder struct {
	stubClient
	err error
}

func (e erringEmbedder) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	return model.Embeddings{}, e.err
}
