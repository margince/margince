// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// recordingEmbedder keeps the inputs a provider would have received.
type recordingEmbedder struct {
	stubClient
	inputs *[]string
}

func (e recordingEmbedder) Embed(_ context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	*e.inputs = append(*e.inputs, req.Inputs...)
	vectors := make([][]float32, len(req.Inputs))
	for i := range vectors {
		vectors[i] = []float32{1, 0}
	}
	return model.Embeddings{Vectors: vectors}, nil
}

// An embedding input is record text, not a request body. A key that only looks
// like an attachment's field is prose, and a credential under it is masked.
func TestNoCredentialInRecordTextReachesTheEmbedder(t *testing.T) {
	const key = "sk-ant-api03-FAKEFAKEFAKEfakefakefake0000"
	inputs := []string{
		`The vendor pasted {"data": "` + key + `"} into the thread.`,
		`{"images": ["` + key + `"]}`,
		`{"url": "data:` + key + `"}`,
		"password=Sup3rSecret!\nsecond line",
	}
	var sent []string
	r := assembleRouter(map[Tier]model.Client{}, recordingEmbedder{inputs: &sent}, ProfileEUHosted, &memMeter{},
		DefaultMonthlyTokens, nil, nil, false, nil)
	if _, err := r.Embed(wsContext(t), model.EmbedRequest{Inputs: inputs}); err != nil {
		t.Fatal(err)
	}
	if len(sent) != len(inputs) {
		t.Fatalf("the embedder received %d inputs, want %d", len(sent), len(inputs))
	}
	for i, input := range sent {
		if strings.Contains(input, key) || strings.Contains(input, "Sup3rSecret") {
			t.Errorf("input %d reached the embedder with its credential: %q", i, input)
		}
	}
}
