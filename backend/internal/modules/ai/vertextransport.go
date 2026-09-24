// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// vertexLocationShape admits the two multi-regions, global, and a region
// name. The host is built from the location, so nothing else may reach it.
var vertexLocationShape = regexp.MustCompile(`^(global|us|eu|[a-z]+-[a-z]+[0-9]{1,2})$`)

// vertexModelShape is a publisher model id. The id is a path segment of
// every model call, so a separator or a query character may not reach it.
var vertexModelShape = regexp.MustCompile(`^[a-z0-9][a-z0-9._@-]{0,199}$`)

// vertexPublisherModelsURL lists Google's publisher models. It is metadata
// only, carries no customer data, and is served from the global host alone.
const vertexPublisherModelsURL = "https://aiplatform.googleapis.com/v1beta1/publishers/google/models"

// vertexGlobal is the location Google may process anywhere.
const vertexGlobal = "global"

// vertexHost is the API host serving one location, which is also where
// Google processes the call. location must already match vertexLocationShape.
func vertexHost(location string) string {
	switch location {
	case "eu", "us":
		return "https://aiplatform." + location + ".rep.googleapis.com"
	case vertexGlobal:
		return "https://aiplatform.googleapis.com"
	default:
		return "https://" + location + "-aiplatform.googleapis.com"
	}
}

// validateVertexPlacement holds the fields a gemini_vertex binding is
// addressed by: a location is required there and meaningless anywhere else,
// and a base_url is refused because the host is derived, never supplied.
func validateVertexPlacement(label string, binding ProviderConfig) error {
	if binding.Provider != providerGeminiVertex {
		if binding.Location != "" {
			return fmt.Errorf("ai: routing config: %s: `location` applies only to gemini_vertex; provider %q is addressed by its base_url", label, binding.Provider)
		}
		return nil
	}
	if binding.BaseURL != "" {
		return fmt.Errorf("ai: routing config: %s: gemini_vertex takes no base_url — its host follows from `location`", label)
	}
	if err := vertexLocationError(label, binding.Location); err != nil {
		return err
	}
	if binding.Model != "" && !vertexModelShape.MatchString(binding.Model) {
		return fmt.Errorf("ai: routing config: %s: gemini_vertex model %q is not a publisher model id such as gemini-3.5-flash", label, binding.Model)
	}
	return nil
}

func vertexLocationError(label, location string) error {
	if !vertexLocationShape.MatchString(location) {
		return fmt.Errorf("ai: routing config: %s: gemini_vertex needs a `location`: eu, us, global, or a region such as europe-west4", label)
	}
	return nil
}

// selectVertex builds the gemini_vertex client. Every call it makes, the
// token exchange included, refuses redirects: each one carries a credential.
//
//nolint:ireturn // SelectBrain's own return shape, so a refusal stays a nil interface
func selectVertex(cfg ProviderConfig, keys config.Lookup, httpc *http.Client) (model.Client, error) {
	keyFile := cloudKey(providerGeminiVertex, keys)
	if keyFile == "" {
		return nil, byokKeyRequired(providerGeminiVertex)
	}
	if err := vertexLocationError("the binding", cfg.Location); err != nil {
		return nil, err
	}
	account, err := parseVertexServiceAccount(keyFile)
	if err != nil {
		return nil, err
	}
	strict := noRedirect(httpc)
	return &geminiClient{
		http: strict,
		transport: vertexTransport{
			host:      vertexHost(cfg.Location),
			projectID: account.projectID,
			location:  cfg.Location,
			tokens:    newVertexTokenSource(account, strict, wallClock{}),
		},
		defaultModel:    cfg.Model,
		attachmentMIMEs: narrowedCarriage(geminiCarries, cfg.Input),
	}, nil
}

// vertexTransport is Gemini on Vertex AI: one project at one location,
// authorised by a service account's access token.
type vertexTransport struct {
	host      string
	projectID string
	location  string
	tokens    *vertexTokenSource
}

func (t vertexTransport) modelURL(model, verb string) string {
	return t.host + "/v1/projects/" + t.projectID + "/locations/" + t.location + "/publishers/google/models/" + url.PathEscape(model) + ":" + verb
}

func (t vertexTransport) modelsURL() string { return vertexPublisherModelsURL }

func (t vertexTransport) authorize(ctx context.Context, r *http.Request) error {
	token, err := t.tokens.accessToken(ctx)
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// readModelPage keeps the Gemini chat and embedding families: the publisher
// catalog also lists image, video and speech models no binding here can call.
// It states no display name and no methods, so the lane is read off the id.
func (t vertexTransport) readModelPage(raw []byte) ([]model.Info, string, error) {
	var page struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"publisherModels"` //nolint:tagliatelle // Google's wire format (camelCase)
		NextPageToken string `json:"nextPageToken"` //nolint:tagliatelle // Google's wire format (camelCase)
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, "", fmt.Errorf("ai: gemini_vertex: decode model list: %w", err)
	}
	models := make([]model.Info, 0, len(page.Models))
	for _, m := range page.Models {
		id := strings.TrimPrefix(m.Name, "publishers/google/models/")
		switch {
		case strings.Contains(id, "embedding"):
			models = append(models, model.Info{ID: id, Lane: model.LaneEmbeddings})
		case strings.HasPrefix(id, "gemini-"):
			models = append(models, model.Info{ID: id, Lane: model.LaneChat})
		}
	}
	return models, page.NextPageToken, nil
}

// wallClock is the production Clock for a token cache built inside
// SelectBrain, which has no caller-supplied clock to take.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }
