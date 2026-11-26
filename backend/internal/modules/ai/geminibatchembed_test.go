// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// batchEmbedServer answers :batchEmbedContents with one vector per request,
// [n, 1] for the request's input "p<n>", and records each call's batch.
func batchEmbedServer(t *testing.T, answerShort bool) (model.Client, *[]geminiBatchEmbedWire) {
	t.Helper()
	var calls []geminiBatchEmbedWire
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models/gemini-embedding-001:batchEmbedContents") {
			t.Errorf("wrong path %s", r.URL.Path)
		}
		var wire geminiBatchEmbedWire
		if err := json.Unmarshal(readBody(t, r.Body), &wire); err != nil {
			t.Errorf("decoding the batch: %v", err)
		}
		calls = append(calls, wire)
		answers := wire.Requests
		if answerShort {
			answers = answers[1:]
		}
		parts := make([]string, len(answers))
		for i, req := range answers {
			parts[i] = fmt.Sprintf(`{"values":[%s,1]}`, strings.TrimPrefix(req.Content.Parts[0].Text, "p"))
		}
		_, _ = w.Write([]byte(`{"embeddings":[` + strings.Join(parts, ",") + `]}`))
	})
	return client, &calls
}

func numberedInputs(n int) []string {
	inputs := make([]string, n)
	for i := range inputs {
		inputs[i] = fmt.Sprintf("p%d", i)
	}
	return inputs
}

func TestGeminiEmbedsSeveralInputsInOneCallInTheirOrder(t *testing.T) {
	client, calls := batchEmbedServer(t, false)
	res, err := client.Embed(context.Background(), model.EmbedRequest{Inputs: numberedInputs(3), Dimensions: 1536})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("three inputs took %d calls, want 1", len(*calls))
	}
	for i, req := range (*calls)[0].Requests {
		if req.Model != "models/gemini-embedding-001" || req.OutputDimensionality != 1536 {
			t.Fatalf("request %d = %+v, want the embed model at width 1536", i, req)
		}
	}
	for i, vec := range res.Vectors {
		if vec[0] != float32(i) {
			t.Fatalf("vector %d is input %v's", i, vec[0])
		}
	}
	if res.Dims != 2 {
		t.Fatalf("dims = %d, want 2", res.Dims)
	}
}

func TestGeminiSplitsABatchPastTheLimitTheAPIAccepts(t *testing.T) {
	client, calls := batchEmbedServer(t, false)
	res, err := client.Embed(context.Background(), model.EmbedRequest{Inputs: numberedInputs(geminiBatchEmbedLimit + 1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || len((*calls)[0].Requests) != geminiBatchEmbedLimit || len((*calls)[1].Requests) != 1 {
		t.Fatalf("%d inputs went out as %d call(s), want %d then 1", geminiBatchEmbedLimit+1, len(*calls), geminiBatchEmbedLimit)
	}
	if last := res.Vectors[geminiBatchEmbedLimit]; last[0] != float32(geminiBatchEmbedLimit) {
		t.Fatalf("the last vector is input %v's, want %d's", last[0], geminiBatchEmbedLimit)
	}
}

// The answer is positional, so an answer short of its inputs is refused rather
// than written against the wrong passages.
func TestGeminiRefusesABatchAnswerShortOfItsInputs(t *testing.T) {
	client, _ := batchEmbedServer(t, true)
	if _, err := client.Embed(context.Background(), model.EmbedRequest{Inputs: numberedInputs(3)}); err == nil {
		t.Fatal("a batch answer short of its inputs was accepted")
	}
}
