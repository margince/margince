// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// passportHoldingEveryScope is a user's own assistant that may call any verb.
func passportHoldingEveryScope() context.Context {
	ctx := context.Background()
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:own", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		SeatType: principal.SeatFull, Scopes: principal.NewScopeSet(mcpInfoScopes()...),
	})
}

// routeVerbArgs fills every argument a verb requires with a value its route
// would bind.
func routeVerbArgs(v agents.RouteVerb) json.RawMessage {
	args := map[string]any{}
	for _, p := range v.PathParams {
		args[p] = ids.NewV7().String()
	}
	fillRequired(args, v.Query)
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return raw
}

// fillRequired gives each required query parameter a value of its type.
func fillRequired(args map[string]any, query []agents.ViewQuery) {
	for _, q := range query {
		switch {
		case !q.Required:
		case len(q.Enum) > 0:
			args[q.Name] = q.Enum[0]
		case q.Type == "integer" || q.Type == "number":
			args[q.Name] = 1
		case q.Type == "boolean":
			args[q.Name] = true
		case q.Format == "uuid":
			args[q.Name] = ids.NewV7().String()
		default:
			args[q.Name] = "1"
		}
	}
}

func trustOf(t *testing.T, sealed json.RawMessage) (string, bool) {
	t.Helper()
	var env agents.Envelope
	if err := json.Unmarshal(sealed, &env); err != nil {
		t.Fatalf("not an envelope: %v (%s)", err, sealed)
	}
	warned := false
	for _, w := range env.Warnings {
		warned = warned || w.Code == "untrusted_content"
	}
	return env.Trust, warned
}

// Every verb the contract serves by its route is sealed at the trust its answer
// schema allows. An answer that may hold text someone wrote is t2 and carries
// the warning, whatever the route returns. So a verb the contract adds is read
// with caution without anyone declaring it.
func TestEveryRouteVerbIsSealedAtTheTrustItsAnswerSchemaAllows(t *testing.T) {
	verbs := declaredRouteVerbs(agentPolicies)
	registry := agents.NewRegistry(nil, auth.NewGate(fullSeat{}))
	agents.RegisterRouteVerbs(registry, verbs, func(context.Context, agents.RouteCall) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	ctx := passportHoldingEveryScope()
	untrusted := map[string]bool{}
	for _, v := range verbs {
		if v.UploadField != "" {
			continue // its answer schema is checked below; a call needs a file
		}
		out, _, err := registry.InvokeServing(ctx, v.Verb, routeVerbArgs(v))
		if err != nil {
			t.Errorf("%s: %v", v.Verb, err)
			continue
		}
		trust, warned := trustOf(t, out)
		free := agents.CarriesFreeText(v.AnswerSchema)
		untrusted[v.Verb] = trust == "t2"
		if free && (trust != "t2" || !warned) {
			t.Errorf("%s answers free text and was sealed trust=%q warned=%v, want t2 with the warning", v.Verb, trust, warned)
		}
		if !free && trust != "t0" {
			t.Errorf("%s answers only ids, numbers, dates and closed words, and was sealed %q, want t0", v.Verb, trust)
		}
	}
	// The answers that carry a correspondent's or a website's words, by name. A
	// schema reader that went blind to them fails here, not in a client.
	for _, verb := range []string{"get_mail_draft", "get_attachment_extraction", "get_anchor_company_site_read", "get_company_scan"} {
		if !untrusted[verb] {
			t.Errorf("%s was not sealed untrusted (served: %v)", verb, untrusted[verb])
		}
	}
	if len(untrusted) < 500 {
		t.Fatalf("only %d route verbs were served; the census is short and this gate proves little", len(untrusted))
	}
}

// read_view serves whatever its route answers, a mail body among them, and no
// view declares a product-made answer.
func TestEveryReadViewIsSealedUntrusted(t *testing.T) {
	views := readViews(agentPolicies)
	registry := agents.NewRegistry(nil, auth.NewGate(fullSeat{}))
	agents.RegisterReadViewTool(registry, views, func(context.Context, agents.ReadView, map[string]string, url.Values) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	ctx := passportHoldingEveryScope()
	spec, _ := registry.Spec(agents.ReadViewTool)
	var published struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &published); err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		args := map[string]any{"view": view.Name}
		for _, p := range view.PathParams {
			args[p] = ids.NewV7().String()
			if published.Properties[p].Type == "integer" {
				args[p] = 2026
			}
		}
		fillRequired(args, view.Query)
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		out, err := registry.Invoke(ctx, agents.ReadViewTool, raw)
		if err != nil {
			t.Errorf("%s: %v", view.Name, err)
			continue
		}
		if trust, warned := trustOf(t, out); trust != "t2" || !warned {
			t.Errorf("view %s was sealed trust=%q warned=%v, want t2 with the warning", view.Name, trust, warned)
		}
	}
	if len(views) < 50 {
		t.Fatalf("only %d views were read; the census is short", len(views))
	}
}
