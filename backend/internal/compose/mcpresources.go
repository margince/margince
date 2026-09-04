// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The resource surface, composed. Three things publish documents now — the
// query vocabulary a module derives per caller, the write vocabulary the agents
// module renders from the contract, and the interactive views the tool surface
// serves — and the transport takes ONE provider.
//
// This is the same composite shape the record surface already uses for
// datasource.SystemOfRecordProvider, and it is here for the same reason:
// deciding which of several providers answers a URI is a composition question,
// and neither module may know the other exists (ADR-0054 §3).

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// mcpResourceProviders is the list of document providers a hosted request
// reaches, in composition order.
//
// It exists as ONE list because both the transport and the gates over it have to
// be talking about the same surface. They were two lists, and the cost was
// exactly the failure a gate is for: a provider added here would not have entered
// the collision check, so a URI it claimed against a view's would have collided in
// production with every gate green and the losing document silently unreachable.
//
// The vocabulary comes FIRST, so a collision resolves to it — see composeResources
// for why the order is stated rather than incidental.
func mcpResourceProviders(capabilities mcp.ResourceProvider, vocabulary mcp.ResourceProvider, views *apps.Provider) []mcp.ResourceProvider {
	// The write vocabulary is unconditional where the query vocabulary is not:
	// it is composed from the contract alone, so it has no pool to be missing
	// and no deployment that can lack it. A record type this build can write is
	// a record type this document can describe.
	// Capabilities is unconditional for the same reason the write vocabulary is:
	// it is derived from the registry this transport already holds, so there is
	// no deployment that can serve tools and fail to describe them.
	// The report plan vocabulary is unconditional for the third time over: it is
	// composed from the engine's prebuilt catalog, which is a compile-time table
	// in this package, and run_report is registered on every build — an overlay
	// system of record refuses it at CALL time, never by withholding the tool.
	// So there is no installation that serves run_report and lacks the
	// vocabulary it refuses against, and a conditionally-absent document would
	// leave the tool naming a document some build does not publish.
	//
	// THE DOOR BESIDE IT *IS* OVERLAY-GUARDED, and the asymmetry is deliberate.
	// It was read as a bug in review, so the reasoning is here rather than in a
	// PR nobody will find:
	//
	//   - A RESOURCE is fetched by a client that asked for it by URI and is
	//     reading documentation. A TOOL call is a step a run spends, and
	//     teaching a name for a verb this workspace refuses spends it for
	//     nothing — which is the whole reason the query vocabulary's door is
	//     guarded too.
	//   - It discloses nothing. The document is the ENGINE's compile-time table,
	//     identical in every installation, so serving it says nothing about this
	//     workspace that run_report's own refusal does not say outright.
	//
	// margince://schema/query splits exactly this way — unguarded resource,
	// guarded door — and it is the precedent this follows.
	// margince://schema/record-fields is NOT a clean precedent for it, though it
	// looks like one: the overlay provider serves no CREATE but does serve some
	// UPDATE, so that document describes verbs a caller may partly still use.
	// This one describes a verb that is refused outright.
	providers := []mcp.ResourceProvider{
		capabilities, vocabulary,
		agents.RecordFieldsResource{},
		agents.NewReportVocabularyResource(reportToolCatalog()),
	}
	// views is nil for a role that composes none — a worker, or an api whose
	// connector gate is off. composeResources drops it, which is the same
	// conditional wiring every other injected capability takes.
	if views == nil {
		return providers
	}
	return append(providers, views)
}

// resourceFanout serves the documents of several providers as one catalogue.
//
// ORDER IS THE CONFLICT RULE, and it is decided here rather than left to
// whichever provider happens to answer first: a read walks the providers in the
// order they were composed and takes the first that serves the URI. Nothing in
// the tree collides today — one provider publishes `margince://`, the other
// `ui://` — and the fitness sweep in this package fails a build where two
// providers claim one URI, so the ordering is a defined tiebreak rather than the
// thing that hides a duplicate.
type resourceFanout struct {
	providers []mcp.ResourceProvider
}

// composeResources fans several providers into one, dropping the nil ones.
//
// A nil provider is an ABSENT capability, not an error: an installation whose
// query vocabulary has no pool, or one with no views, composes the rest and
// serves them. That is the same conditional-wiring rule the tool registrations
// take, and it is what keeps a partial deployment from being an unstartable one.
//
//nolint:ireturn // the seam IS the return type: this answers with one provider, a fan-out over several, or nil for none, and the transport takes the interface
func composeResources(providers ...mcp.ResourceProvider) mcp.ResourceProvider {
	wired := make([]mcp.ResourceProvider, 0, len(providers))
	for _, p := range providers {
		if p != nil {
			wired = append(wired, p)
		}
	}
	if len(wired) == 0 {
		// No provider at all is answered by the transport as an empty
		// catalogue, which it already does for a nil provider — so returning
		// nil here keeps that one path rather than adding an empty composite
		// that behaves identically and has to be reasoned about separately.
		return nil
	}
	if len(wired) == 1 {
		// One provider needs no fan-out. Returning it directly means the
		// composite is only ever in the graph when it is doing something, which
		// keeps a stack trace honest about what is between the transport and the
		// document.
		return wired[0]
	}
	return resourceFanout{providers: wired}
}

// Resources concatenates what each provider publishes to THIS caller. Each
// applies its own narrowing first — the vocabulary is derived per principal, the
// views are the same for everyone — and the transport's scope filter runs on top
// of the result, so a document reaches a client only if its own provider
// published it and the caller's scopes admit it.
func (f resourceFanout) Resources(ctx context.Context) []mcp.Resource {
	var all []mcp.Resource
	for _, p := range f.providers {
		all = append(all, p.Resources(ctx)...)
	}
	return all
}

// ReadResource answers from the first provider that serves the URI.
//
// A not-found from one provider is a reason to ask the NEXT one, and any other
// error is not: a pool fault while reading the vocabulary must not degrade into
// "no such document", because the caller would then be told a document does not
// exist on the strength of a failure to look. So the walk continues only on the
// declared not-found sentinel, and every other error stops it.
func (f resourceFanout) ReadResource(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	for _, p := range f.providers {
		contents, err := p.ReadResource(ctx, uri)
		switch {
		case err == nil:
			return contents, nil
		case errors.Is(err, apperrors.ErrNotFound):
			continue
		default:
			return mcp.ResourceContents{}, fmt.Errorf("compose: reading resource %s: %w", uri, err)
		}
	}
	return mcp.ResourceContents{}, apperrors.ErrNotFound
}

var _ mcp.ResourceProvider = resourceFanout{}
