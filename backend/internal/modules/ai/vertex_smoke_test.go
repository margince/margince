// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build livesmoke

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A live pass over every Vertex call the product makes, against Google: the
// token exchange, a structured completion, a stream, an embedding, the model
// list, the location list, and the probe in both answers. A reachability
// check, not a gate — the httptest suite holds the behaviour. Manual, so the
// unit lane stays hermetic: point MARGINCE_VERTEX_SA_FILE at a key file whose
// account holds roles/aiplatform.user, then
// `go test -tags livesmoke -run TestVertexLiveSmoke ./internal/modules/ai`.
//
// The models are the ones config/presets/gemini_vertex_eu.yaml binds, at the
// locations it binds them.
func TestVertexLiveSmoke(t *testing.T) {
	path := os.Getenv("MARGINCE_VERTEX_SA_FILE")
	if path == "" {
		// Fatal, not Skip: this tag is asked for by hand, and a skip would read as a pass.
		t.Fatal("MARGINCE_VERTEX_SA_FILE is unset — point it at the service-account key file")
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- an operator-named key file, read by a manual test
	if err != nil {
		t.Fatal(err)
	}
	keys := config.Static(map[string]string{cloudKeyEnv[providerGeminiVertex]: string(raw)})
	at := func(location string) *geminiClient {
		t.Helper()
		client, err := SelectBrain(ProviderConfig{Provider: providerGeminiVertex, Location: location}, keys)
		if err != nil {
			t.Fatal(err)
		}
		gemini, ok := client.(*geminiClient)
		if !ok {
			t.Fatalf("SelectBrain built %T, want the Gemini client", client)
		}
		return gemini
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eu := at("eu")
	resp, err := eu.Complete(ctx, model.Request{
		Model:          "gemini-3.5-flash",
		Messages:       []model.Message{{Role: roleUser, Content: "Name one primary colour."}},
		ResponseSchema: json.RawMessage(`{"type":"object","properties":{"colour":{"type":"string"}},"required":["colour"]}`),
		MaxTokens:      256,
		SecretStripper: NewSecretStripper(),
	})
	if err != nil {
		t.Fatalf("structured complete at eu: %v", err)
	}
	var answer struct{ Colour string }
	if err := json.Unmarshal([]byte(resp.Text), &answer); err != nil || answer.Colour == "" {
		t.Fatalf("the structured answer %q did not follow the schema: %v", resp.Text, err)
	}

	stream, err := eu.Stream(ctx, model.Request{
		Model:          "gemini-3.1-flash-lite",
		Messages:       []model.Message{{Role: roleUser, Content: "Count from one to five."}},
		MaxTokens:      128,
		SecretStripper: NewSecretStripper(),
	})
	if err != nil {
		t.Fatalf("stream at eu: %v", err)
	}
	var streamed strings.Builder
	for {
		chunk, more, err := stream.Next(ctx)
		if err != nil {
			t.Fatalf("stream read: %v", err)
		}
		if !more {
			break
		}
		streamed.WriteString(chunk)
	}
	if err := stream.Close(); err != nil || streamed.Len() == 0 {
		t.Fatalf("stream read %q, close %v", streamed.String(), err)
	}

	embedded, err := at("europe-west4").Embed(ctx, model.EmbedRequest{
		Model: "gemini-embedding-001", Inputs: []string{"margin", "revenue"}, Dimensions: 1536,
	})
	if err != nil || embedded.Dims != 1536 || len(embedded.Vectors) != 2 {
		t.Fatalf("embed at europe-west4: dims %d, %d vectors, %v", embedded.Dims, len(embedded.Vectors), err)
	}

	models, err := eu.ListModels(ctx)
	if err != nil || len(models) == 0 {
		t.Fatalf("model list: %d models, %v", len(models), err)
	}
	metadata, ok := vertexOf(at(vertexMetadataLocation))
	if !ok {
		t.Fatal("the metadata client is not a Vertex client")
	}
	locations, err := metadata.listLocations(ctx)
	// Google names no region, so the id's presence is the answer.
	if _, listed := locations["europe-west4"]; err != nil || !listed {
		t.Fatalf("location list: %v, %v", locations, err)
	}

	if err := eu.probeModel(ctx, "gemini-3.5-flash", model.LaneChat); err != nil {
		t.Errorf("probe of a served model: %v", err)
	}
	if err := eu.probeModel(ctx, "gemini-embedding-001", model.LaneEmbeddings); !errors.Is(err, errModelNotFound) {
		t.Errorf("probe of an embedder eu does not serve = %v, want errModelNotFound", err)
	}
	t.Logf("live smoke ok: %q, %d streamed bytes, %d models, %d locations", resp.Text, streamed.Len(), len(models), len(locations))
}
